package agent

import (
	"log"
)

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

// SendEncrypted encrypts raw IP packet and relays via control plane to target.
func (t *Tunnel) SendEncrypted(targetNodeID uint, packet []byte) error {
	encrypted, err := t.crypto.Encrypt(packet)
	if err != nil {
		return err
	}
	frame := EncodeFrame(targetNodeID, encrypted)
	return t.agent.ws.RelayTo(targetNodeID, frame)
}

// ReceiveEncrypted receives encrypted data and decrypts it.
func (t *Tunnel) ReceiveEncrypted(fromNodeID uint, frame []byte) ([]byte, error) {
	_, _, payload, err := DecodeFrame(frame)
	if err != nil {
		return nil, err
	}
	plaintext, err := t.crypto.Decrypt(payload)
	if err != nil {
		log.Printf("tunnel decrypt failed from node %d: %v", fromNodeID, err)
		return nil, err
	}
	return plaintext, nil
}

// Run starts the packet forwarding loop from TUN to tunnel.
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
				continue
			}
			if err := t.SendEncrypted(nextHop, packet); err != nil {
				log.Printf("tunnel send to node %d failed: %v", nextHop, err)
			}
		}
	}()
}
