package agent

import (
	"log"
)

// Tunnel manages encrypted data transport over peer connections.
type Tunnel struct {
	agent  *Agent
	crypto *Crypto
}

func NewTunnel(agent *Agent) *Tunnel {
	return &Tunnel{
		agent:  agent,
		crypto: agent.crypto,
	}
}

// SendEncrypted encrypts raw IP packet and sends it to a peer.
func (t *Tunnel) SendEncrypted(nodeID uint, packet []byte) error {
	encrypted, err := t.crypto.Encrypt(packet)
	if err != nil {
		return err
	}

	frame := EncodeFrame(nodeID, encrypted)
	return t.agent.peers.SendRaw(nodeID, frame)
}

// ReceiveEncrypted receives encrypted data from a peer and decrypts it.
func (t *Tunnel) ReceiveEncrypted(nodeID uint, frame []byte) ([]byte, error) {
	_, _, payload, err := DecodeFrame(frame)
	if err != nil {
		return nil, err
	}

	plaintext, err := t.crypto.Decrypt(payload)
	if err != nil {
		log.Printf("tunnel decrypt failed from node %d: %v", nodeID, err)
		return nil, err
	}

	return plaintext, nil
}

// Run starts the packet forwarding loop from TUN to peers.
func (t *Tunnel) Run() {
	go func() {
		for {
			packet, err := t.agent.tun.Read()
			if err != nil || len(packet) == 0 {
				continue
			}

			// Determine target node from IP header
			targetIP := extractDstIP(packet)
			targetNode := t.agent.routes.Lookup(targetIP)
			if targetNode == 0 {
				continue // no route for this destination
			}

			if err := t.SendEncrypted(targetNode, packet); err != nil {
				log.Printf("tunnel send to node %d failed: %v", targetNode, err)
			}
		}
	}()
}

// extractDstIP extracts the destination IP from an IPv4 packet header.
func extractDstIP(packet []byte) string {
	if len(packet) < 20 {
		return ""
	}
	dst := packet[16:20]
	return byteToIP(dst)
}

func byteToIP(b []byte) string {
	if len(b) != 4 {
		return ""
	}
	_ = b[0]
	_ = b[1]
	_ = b[2]
	_ = b[3]
	return itoa(int(b[0])) + "." + itoa(int(b[1])) + "." + itoa(int(b[2])) + "." + itoa(int(b[3]))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
