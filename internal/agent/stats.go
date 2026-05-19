package agent

import (
	"sync"
	"time"
)

// StatsCollector gathers traffic stats and reports to control plane.
type StatsCollector struct {
	agent    *Agent
	onReport func(tunnelName string, rx, tx uint64) // for testing
	rx       map[uint]uint64
	tx       map[uint]uint64
	mu       sync.Mutex
}

func NewStatsCollector(agent *Agent) *StatsCollector {
	return &StatsCollector{
		agent: agent,
		rx:    make(map[uint]uint64),
		tx:    make(map[uint]uint64),
	}
}

// Run periodically sends stats to the control plane.
func (s *StatsCollector) Run(quit <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
			s.report()
		}
	}
}

func (s *StatsCollector) RecordRX(nodeID uint, bytes uint64) {
	s.mu.Lock()
	s.rx[nodeID] += bytes
	s.mu.Unlock()
}

func (s *StatsCollector) RecordTX(nodeID uint, bytes uint64) {
	s.mu.Lock()
	s.tx[nodeID] += bytes
	s.mu.Unlock()
}

type tunnelStatEntry struct {
	RX        uint64  `json:"rx"`
	TX        uint64  `json:"tx"`
	LatencyMs float64 `json:"latency_ms"`
}

func (s *StatsCollector) report() {
	if s.agent.ws == nil || !s.agent.ws.IsConnected() {
		return
	}

	tunnels := make(map[string]tunnelStatEntry)

	s.mu.Lock()
	// Collect all nodeIDs that have either RX or TX traffic in this window
	allKeys := make(map[uint]bool, len(s.rx)+len(s.tx))
	for k := range s.rx {
		allKeys[k] = true
	}
	for k := range s.tx {
		allKeys[k] = true
	}

	for nodeID := range allKeys {
		rx := s.rx[nodeID]
		tx := s.tx[nodeID]
		if rx == 0 && tx == 0 {
			delete(s.rx, nodeID)
			delete(s.tx, nodeID)
			continue
		}
		key := "tun-" + itoaUint(nodeID)
		tunnels[key] = tunnelStatEntry{RX: rx, TX: tx}
		delete(s.rx, nodeID)
		delete(s.tx, nodeID)
	}
	s.mu.Unlock()

	// Add latency info from probe
	if s.agent.probe != nil {
		for nodeID, lat := range s.agent.probe.Latencies() {
			key := "tun-" + itoaUint(nodeID)
			if t, ok := tunnels[key]; ok {
				t.LatencyMs = lat
				tunnels[key] = t
			}
		}
	}

	msg := map[string]any{
		"type": "stats",
		"data": map[string]any{
			"tunnels": tunnels,
			"system": map[string]any{
				"cpu_pct": readCPU(),
				"mem_mb":  readMemMB(),
			},
		},
	}

	s.agent.ws.SendJSON(msg)

	// Test callback
	if s.onReport != nil {
		for k, v := range tunnels {
			s.onReport(k, v.RX, v.TX)
		}
	}
}

func itoaUint(n uint) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(byte('0'+n%10)) + result
		n /= 10
	}
	return result
}
