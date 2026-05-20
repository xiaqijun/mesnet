package agent

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"sort"
	"sync/atomic"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

// KeyPair holds a Curve25519 key pair for Noise-based session establishment.
type KeyPair struct {
	PrivateKey []byte // 32 bytes, clamped
	PublicKey  []byte // 32 bytes
}

// GenerateKeyPair creates a new Curve25519 keypair from crypto/rand.
func GenerateKeyPair() (*KeyPair, error) {
	priv := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, priv); err != nil {
		return nil, err
	}
	// Clamp for Curve25519 (required by RFC 7748)
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return nil, err
	}
	return &KeyPair{PrivateKey: priv, PublicKey: pub}, nil
}

// LoadOrGenerateKeyPair loads a keypair from file, or generates and saves it.
func LoadOrGenerateKeyPair(keyPath string) (*KeyPair, error) {
	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) == 64 {
		priv := data[:32]
		pub := data[32:]
		return &KeyPair{PrivateKey: priv, PublicKey: pub}, nil
	}

	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	// Save to file (ignoring errors — ephemeral key is OK)
	saved := append([]byte{}, kp.PrivateKey...)
	saved = append(saved, kp.PublicKey...)
	os.WriteFile(keyPath, saved, 0600)

	return kp, nil
}

// SecureChannel implements an encrypted session between two agents.
//
// Handshake (1-RTT, similar to Noise XK):
//
//	Initiator                           Responder
//	   │                                    │
//	   │── Frame{FlagHandshake, E_i} ──────►│
//	   │                                    │  compute shared secret
//	   │◄── Frame{FlagHandshake, E_r} ──────│
//	   │  compute shared secret              │
//	   │══ Encrypted{FlagData, seq} ════════│
//
// Shared secret derivation (Mix of all 4 DH operations):
//
//	dh1 = X25519(local_eph, remote_eph)        // eph-ephemeral (PFS)
//	dh2 = X25519(local_static, remote_eph)     // static-ephemeral
//	dh3 = X25519(local_eph, remote_static)     // eph-static
//	dh4 = X25519(local_static, remote_static)  // static-static (auth)
//	key = SHA256(dh1 || dh2 || dh3 || dh4 || "mesnet-v2")
type SecureChannel struct {
	localStatic  *KeyPair
	remoteStatic []byte    // peer's static public key (32 bytes)
	ephemeralKey *KeyPair  // temporary, wiped after handshake

	sessionKey [32]byte // derived after successful handshake
	aead       cipher.AEAD

	seqOut atomic.Uint64 // sender sequence counter
	seqIn  atomic.Uint64 // highest seen seq (for anti-replay, optional window)

	established bool
}

// NewSecureChannel creates a channel with local key material.
// remoteStaticPub can be nil for the responder until handshake completes.
func NewSecureChannel(local *KeyPair, remoteStaticPub []byte) *SecureChannel {
	return &SecureChannel{
		localStatic:  local,
		remoteStatic: remoteStaticPub,
	}
}

// ErrHandshakeFailed is returned when the Noise handshake fails.
var ErrHandshakeFailed = errors.New("secure channel handshake failed")

// ErrReplay is returned when a duplicate or out-of-window sequence number is detected.
var ErrReplay = errors.New("potential replay attack detected")

// ErrNotEstablished is returned when sending data before handshake completes.
var ErrNotEstablished = errors.New("secure channel not established")

// InitiateHandshake sends the initiator's ephemeral key and returns the frame bytes
// to send to the responder. The caller must send this frame over the transport.
func (sc *SecureChannel) InitiateHandshake() (frame []byte, err error) {
	if sc.established {
		return nil, errors.New("channel already established")
	}

	ephKey, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	frame = EncodeFrame(FlagHandshake, 0, 0, ephKey.PublicKey)
	sc.ephemeralKey = ephKey

	return frame, nil
}

// CompleteHandshake processes the responder's reply and derives the session key.
func (sc *SecureChannel) CompleteHandshake(remoteEph []byte) error {
	if sc.ephemeralKey == nil {
		return ErrHandshakeFailed
	}
	if len(remoteEph) != 32 {
		return ErrHandshakeFailed
	}

	key := computeSharedSecret(
		sc.localStatic.PrivateKey,
		sc.ephemeralKey.PrivateKey,
		sc.remoteStatic,
		remoteEph,
	)
	// Keep ephemeral key for potential re-handshakes from the peer

	return sc.installKey(key[:])
}

// AcceptHandshake processes the initiator's handshake frame, derives the session key,
// and returns the responder's ephemeral public key frame to send back.
func (sc *SecureChannel) AcceptHandshake(remoteEph []byte) (response []byte, err error) {
	if len(remoteEph) != 32 {
		return nil, ErrHandshakeFailed
	}

	// Generate responder's ephemeral key
	ephKey, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	key := computeSharedSecret(
		sc.localStatic.PrivateKey,
		ephKey.PrivateKey,
		sc.remoteStatic,
		remoteEph,
	)

	if err := sc.installKey(key[:]); err != nil {
		return nil, err
	}
	sc.ephemeralKey = ephKey // keep for re-handshakes

	// Response frame with responder's ephemeral key
	return EncodeFrame(FlagHandshake, 0, 0, ephKey.PublicKey), nil
}

// computeSharedSecret derives the session key from 4 DH operations.
// Both sides call this with (my_static, my_eph, remote_static, remote_eph)
// and get the same result.
//
//	shared = SHA256(
//	    X25519(my_eph, remote_eph) ||
//	    X25519(my_static, remote_eph) ||
//	    X25519(my_eph, remote_static) ||
//	    X25519(my_static, remote_static) ||
//	    "mesnet-v2-session"
//	)
func computeSharedSecret(myStaticPriv, myEphPriv, remoteStaticPub, remoteEphPub []byte) [32]byte {
	dh1, _ := curve25519.X25519(myEphPriv, remoteEphPub)
	dh2, _ := curve25519.X25519(myStaticPriv, remoteEphPub)
	dh3, _ := curve25519.X25519(myEphPriv, remoteStaticPub)
	dh4, _ := curve25519.X25519(myStaticPriv, remoteStaticPub)

	// Sort DH outputs for consistent ordering regardless of initiator/responder role.
	// Without sorting, dh2 and dh3 swap places depending on which side calls this,
	// producing different hash inputs and mismatched session keys.
	dh := [][]byte{dh1, dh2, dh3, dh4}
	sort.Slice(dh, func(i, j int) bool {
		return bytes.Compare(dh[i], dh[j]) < 0
	})

	h := sha256.New()
	for _, d := range dh {
		h.Write(d)
	}
	h.Write([]byte("mesnet-v2-session"))

	var key [32]byte
	copy(key[:], h.Sum(nil)[:32])
	return key
}

// installKey sets up the AEAD cipher with the derived session key.
func (sc *SecureChannel) installKey(key []byte) error {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return err
	}
	copy(sc.sessionKey[:], key)
	sc.aead = aead
	sc.seqIn.Store(0)
	sc.seqOut.Store(0)
	sc.established = true
	return nil
}

// Encrypt encrypts and authenticates plaintext using ChaCha20-Poly1305.
// The sequence number is embedded in the nonce for replay protection.
// Returns ciphertext with 16-byte AEAD tag appended.
func (sc *SecureChannel) Encrypt(plaintext []byte) ([]byte, uint32) {
	seq := uint32(sc.seqOut.Add(1) - 1)
	return sc.encryptWithSeq(seq, plaintext), seq
}

// encryptWithSeq performs encryption with a specific sequence number.
func (sc *SecureChannel) encryptWithSeq(seq uint32, plaintext []byte) []byte {
	// ChaCha20-Poly1305 nonce is 12 bytes.
	// We use the upper 4 bytes for the sequence number, lower 8 bytes zero.
	var nonce [12]byte
	nonce[0] = byte(seq >> 24)
	nonce[1] = byte(seq >> 16)
	nonce[2] = byte(seq >> 8)
	nonce[3] = byte(seq)

	return sc.aead.Seal(nil, nonce[:], plaintext, nil)
}

// Decrypt verifies and decrypts a ciphertext using ChaCha20-Poly1305.
// Returns plaintext on success, or error if authentication fails.
func (sc *SecureChannel) Decrypt(seq uint32, ciphertext []byte) ([]byte, error) {
	if !sc.established {
		return nil, ErrNotEstablished
	}

	// Basic anti-replay: reject old sequence numbers
	if uint64(seq) < sc.seqIn.Load() {
		// Allow some out-of-order (sliding window, up to 32 packets behind)
		if uint64(seq)+32 < sc.seqIn.Load() {
			return nil, ErrReplay
		}
	}

	var nonce [12]byte
	nonce[0] = byte(seq >> 24)
	nonce[1] = byte(seq >> 16)
	nonce[2] = byte(seq >> 8)
	nonce[3] = byte(seq)

	plaintext, err := sc.aead.Open(nil, nonce[:], ciphertext, nil)
	if err != nil {
		return nil, err
	}

	// Update seqIn (atomic, handles concurrent decrypts)
	for {
		current := sc.seqIn.Load()
		if uint64(seq) <= current {
			break
		}
		if sc.seqIn.CompareAndSwap(current, uint64(seq)) {
			break
		}
	}

	return plaintext, nil
}

// IsEstablished returns true after a successful handshake.
func (sc *SecureChannel) IsEstablished() bool {
	return sc.established
}

// IsEstablishing returns true if handshake is in progress.
// This indicates the channel was created but handshake not yet complete.
func (sc *SecureChannel) IsEstablishing() bool {
	return !sc.established
}

// SetRemoteStatic sets the peer's static public key.
// Used by the responder who receives it as part of the incoming connection.
func (sc *SecureChannel) SetRemoteStatic(pub []byte) {
	if len(pub) == 32 {
		sc.remoteStatic = pub
	}
}

// GetEphemeralPublicKey returns the stored ephemeral public key,
// or nil if not available.
func (sc *SecureChannel) GetEphemeralPublicKey() []byte {
	if sc.ephemeralKey != nil {
		return sc.ephemeralKey.PublicKey
	}
	return nil
}

// Wipe clears all key material from the channel.
// Called when a session is terminated.
func (sc *SecureChannel) Wipe() {
	for i := range sc.sessionKey {
		sc.sessionKey[i] = 0
	}
	sc.aead = nil
	sc.established = false
	sc.ephemeralKey = nil
}

// Ensure the key size constants are clear
const (
	KeySize         = 32 // Curve25519 key size in bytes
	NonceSize       = 12 // ChaCha20-Poly1305 nonce size
	AEADTagSize     = 16 // ChaCha20-Poly1305 authentication tag
	MaxSequenceSkip = 32 // Maximum out-of-order packets allowed
)
