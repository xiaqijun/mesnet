package agent

import (
	"sync"
	"time"
)

// Probe measures latency to connected peers.
type Probe struct {
	peers    *PeerManager
	latency  map[uint]float64
	mu       sync.RWMutex
}

func NewProbe(pm *PeerManager) *Probe {
	return &Probe{
		peers:   pm,
		latency: make(map[uint]float64),
	}
}

// Run periodically probes all connected peers.
func (p *Probe) Run(quit <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
			p.probeAll()
		}
	}
}

func (p *Probe) probeAll() {
	peers := p.peers.ListPeers()
	for _, nodeID := range peers {
		start := time.Now()

		// Send a tiny binary probe frame
		frame := EncodeFrame(FlagProbe, 0, uint64(nodeID), []byte{0x00})
		if err := p.peers.SendRaw(nodeID, frame); err != nil {
			continue
		}

		elapsed := time.Since(start).Seconds() * 1000 // ms

		p.mu.Lock()
		p.latency[nodeID] = elapsed
		p.mu.Unlock()
	}
}

// Latencies returns current latency measurements.
func (p *Probe) Latencies() map[uint]float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make(map[uint]float64, len(p.latency))
	for k, v := range p.latency {
		result[k] = v
	}
	return result
}
