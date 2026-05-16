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
// Supports multi-hop relay: if the next-hop differs from the destination node,
// the packet is relayed through the next-hop peer.
func (t *Tunnel) Run() {
	go func() {
		for {
			packet, err := t.agent.tun.Read()
			if err != nil || len(packet) < 20 {
				continue
			}

			dstIP := extractDstIP(packet)
			_, nextHop := t.agent.routes.Lookup(dstIP)
			if nextHop == 0 {
				continue // no route
			}

			if err := t.SendEncrypted(nextHop, packet); err != nil {
				log.Printf("tunnel send to node %d failed: %v", nextHop, err)
			}
		}
	}()
}
