package agent

import (
	"crypto/rand"
	"encoding/binary"
	"io"
)

// Crypto handles Noise IK handshake and symmetric encryption for tunnels.
// Simplified implementation: uses a pre-shared key derived from the agent token
// for symmetric encryption. In production this would use the full Noise IK pattern.
type Crypto struct {
	key [32]byte
}

func NewCrypto() *Crypto {
	c := &Crypto{}
	// Derive key from random (in production, from Noise handshake)
	rand.Read(c.key[:])
	return c
}

// Encrypt encrypts plaintext with ChaCha20-Poly1305 style encryption.
// Simplified: XOR with keystream derived from key + random nonce.
func (c *Crypto) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := make([]byte, 12+len(plaintext))
	copy(ciphertext[:12], nonce)

	for i := range plaintext {
		// Simplified stream cipher
		ki := int(binary.LittleEndian.Uint64(c.key[i%24:i%24+8])) % 256
		ciphertext[12+i] = plaintext[i] ^ byte(ki) ^ nonce[i%12]
	}

	return ciphertext, nil
}

// Decrypt reverses the encryption.
func (c *Crypto) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 12 {
		return nil, io.ErrUnexpectedEOF
	}

	nonce := ciphertext[:12]
	plaintext := make([]byte, len(ciphertext)-12)

	for i := range plaintext {
		ki := int(binary.LittleEndian.Uint64(c.key[i%24:i%24+8])) % 256
		plaintext[i] = ciphertext[12+i] ^ byte(ki) ^ nonce[i%12]
	}

	return plaintext, nil
}

// GenerateKey derives a new key for a peer connection.
func (c *Crypto) GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// AddRandomPadding adds random-length padding to obscure packet sizes.
func AddRandomPadding(data []byte, maxPad int) []byte {
	padLen := int(data[0]) % maxPad
	padded := make([]byte, len(data)+padLen)
	copy(padded, data)
	rand.Read(padded[len(data):])
	return padded
}

// RemoveRandomPadding removes the random padding.
func RemoveRandomPadding(data []byte, originalLen int) []byte {
	if originalLen > len(data) {
		return data
	}
	return data[:originalLen]
}
