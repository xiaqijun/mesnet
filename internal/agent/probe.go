package agent

import (
	"sync"
	"sync/atomic"
	"time"
)

// Probe measures RTT (round-trip time) to connected peers using
// echo probe/response frames with sequence numbers for async tracking.
type Probe struct {
	peers   *PeerManager
	latency map[uint]float64   // peer → latency in ms
	pending map[uint32]int64   // seq → send time (unix nano)
	seq     atomic.Uint32      // probe sequence counter
	mu      sync.RWMutex
}

func NewProbe(pm *PeerManager) *Probe {
	p := &Probe{
		peers:   pm,
		latency: make(map[uint]float64),
		pending: make(map[uint32]int64),
	}

	// Register probe response handler
	pm.SetOnProbeResponse(func(nodeID uint, seq uint32) {
		p.handleResponse(nodeID, seq)
	})

	return p
}

// handleResponse is called from readLoop when a probe response arrives.
func (p *Probe) handleResponse(nodeID uint, seq uint32) {
	p.mu.Lock()
	startNano, ok := p.pending[seq]
	if ok {
		delete(p.pending, seq)
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	elapsed := float64(time.Now().UnixNano()-startNano) / 1e6 // ms
	p.mu.Lock()
	p.latency[nodeID] = elapsed
	p.mu.Unlock()
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
		seq := p.seq.Add(1)
		p.mu.Lock()
		p.pending[seq] = time.Now().UnixNano()
		p.mu.Unlock()

		// Send probe frame — peer will echo back with FlagProbeResponse
		frame := EncodeFrame(FlagProbe, seq, uint64(nodeID), nil)
		if err := p.peers.SendRaw(nodeID, frame); err != nil {
			p.mu.Lock()
			delete(p.pending, seq)
			p.mu.Unlock()
			continue
		}
	}

	// Clean stale pending probes (>10s old)
	now := time.Now().UnixNano()
	p.mu.Lock()
	for seq, start := range p.pending {
		if now-start > 10e9 {
			delete(p.pending, seq)
		}
	}
	p.mu.Unlock()
}

// Latencies returns current latency measurements in milliseconds.
func (p *Probe) Latencies() map[uint]float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make(map[uint]float64, len(p.latency))
	for k, v := range p.latency {
		result[k] = v
	}
	return result
}
