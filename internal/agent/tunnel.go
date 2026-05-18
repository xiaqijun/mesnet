package agent

import (
	"log"
	"time"
)

// Tunnel manages encrypted data transport over peer connections.
type Tunnel struct {
	agent *Agent
}

func NewTunnel(agent *Agent) *Tunnel {
	return &Tunnel{agent: agent}
}

// SendEncrypted encrypts a raw IP packet and sends it to a peer using
// the SecureChannel's ChaCha20-Poly1305 AEAD encryption.
func (t *Tunnel) SendEncrypted(nodeID uint, packet []byte) error {
	t.agent.mu.Lock()
	ch := t.agent.channels[nodeID]
	t.agent.mu.Unlock()
	if ch == nil || !ch.IsEstablished() {
		// Fallback to old crypto for backward compat during handshake
		return t.sendLegacy(nodeID, packet)
	}

	encrypted, seq := ch.Encrypt(packet)
	frame := EncodeFrame(FlagData, seq, uint64(nodeID), encrypted)
	return t.agent.router.SendTo(nodeID, frame)
}

// sendLegacy uses the old Crypto for encryption during transition.
func (t *Tunnel) sendLegacy(nodeID uint, packet []byte) error {
	encrypted, err := t.agent.crypto.Encrypt(packet)
	if err != nil {
		return err
	}
	frame := EncodeFrame(FlagData, 0, uint64(nodeID), encrypted)
	return t.agent.router.SendTo(nodeID, frame)
}

// ReceiveEncrypted decrypts a data frame from a peer using
// the SecureChannel's AEAD decryption with anti-replay protection.
func (t *Tunnel) ReceiveEncrypted(frame []byte) ([]byte, error) {
	hdr, payload, err := DecodeFrameHeader(frame)
	if err != nil {
		return nil, err
	}

	if hdr.Flags&FlagData == 0 && hdr.Flags&FlagProbe == 0 {
		return nil, nil // skip non-data frames
	}

	// Try SecureChannel first (with seq-based anti-replay AEAD decrypt)
	nodeID := uint(hdr.NodeID)
	t.agent.mu.Lock()
	ch := t.agent.channels[nodeID]
	t.agent.mu.Unlock()

	if ch != nil && ch.IsEstablished() {
		plaintext, err := ch.Decrypt(hdr.Seq, payload)
		if err != nil {
			log.Printf("tunnel AEAD decrypt failed from node %d seq=%d: %v", nodeID, hdr.Seq, err)
			return nil, err
		}
		return plaintext, nil
	}

	// Fallback to legacy decrypt
	return t.agent.crypto.Decrypt(payload)
}

// Run starts the packet forwarding loop from TUN to peers.
// Each packet is encrypted via SecureChannel and sent through the PacketRouter's
// per-peer async send queues for backpressure isolation.
func (t *Tunnel) Run() {
	go func() {
		// Wait for TUN device to be created by tun_setup command
		for !t.agent.tun.IsUp() {
			time.Sleep(500 * time.Millisecond)
		}

		for {
			packet, err := t.agent.tun.Read()
			if err != nil || len(packet) < 20 {
				if err != nil {
					log.Printf("tunnel: TUN read error: %v", err)
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}

			dstIP := extractDstIP(packet)
			_, nextHop := t.agent.routes.Lookup(dstIP)
			if nextHop == 0 {
				continue
			}

			if err := t.SendEncrypted(nextHop, packet); err != nil {
				log.Printf("tunnel send to node %d failed: %v", nextHop, err)
			}
		}
	}()
}
