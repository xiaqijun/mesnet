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

// SendEncrypted encrypts raw IP packet and sends it to a peer via the
// async PacketRouter (non-blocking, per-peer buffered queue).
func (t *Tunnel) SendEncrypted(nodeID uint, packet []byte) error {
	encrypted, err := t.crypto.Encrypt(packet)
	if err != nil {
		return err
	}
	frame := EncodeFrame(FlagData, 0, uint64(nodeID), encrypted)
	return t.agent.router.SendTo(nodeID, frame)
}

// ReceiveEncrypted receives encrypted data from a peer and decrypts it.
func (t *Tunnel) ReceiveEncrypted(frame []byte) ([]byte, error) {
	hdr, payload, err := DecodeFrameHeader(frame)
	if err != nil {
		return nil, err
	}
	if hdr.Flags&FlagData == 0 && hdr.Flags&FlagProbe == 0 {
		return nil, nil // not data, skip
	}
	plaintext, err := t.crypto.Decrypt(payload)
	if err != nil {
		log.Printf("tunnel decrypt failed: %v", err)
		return nil, err
	}
	return plaintext, nil
}

// Run starts the packet forwarding loop from TUN to peers.
// Uses PacketRouter for async, non-blocking sends.
func (t *Tunnel) Run() {
	go func() {
		for {
			packet, err := t.agent.tun.Read()
			if err != nil || len(packet) < 20 {
				continue
			}

			dstIP := extractDstIP(packet)
			if err := t.agent.router.Route(dstIP, packet); err != nil {
				if err != ErrNoRoute {
					log.Printf("tunnel route to %s failed: %v", dstIP, err)
				}
				continue
			}
		}
	}()
}
