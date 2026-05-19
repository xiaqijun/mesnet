package services

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

type tunnelStats struct {
	RxBytes   int64   `json:"rx"`
	TxBytes   int64   `json:"tx"`
	LatencyMs float64 `json:"latency_ms"`
}

type statsMessage struct {
	Tunnels map[string]tunnelStats `json:"tunnels"`
	System  struct {
		CPUPercent float64 `json:"cpu_pct"`
		MemUsedMB  int     `json:"mem_mb"`
	} `json:"system"`
}

type snapshotEntry struct {
	tunnelID  uint
	rxBytes   int64
	txBytes   int64
	latencyMs float64
}

type snapshotBatcher struct {
	buffer map[uint][]snapshotEntry
	mu     sync.Mutex
}

func (b *snapshotBatcher) add(_ uint, stats statsMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for tunnelIDStr, ts := range stats.Tunnels {
		var tunnelID uint
		fmt.Sscanf(tunnelIDStr, "tun-%d", &tunnelID)
		if tunnelID == 0 {
			continue
		}
		b.buffer[tunnelID] = append(b.buffer[tunnelID], snapshotEntry{
			tunnelID:  tunnelID,
			rxBytes:   ts.RxBytes,
			txBytes:   ts.TxBytes,
			latencyMs: ts.LatencyMs,
		})
	}
}

func (b *snapshotBatcher) flush(db *gorm.DB) {
	b.mu.Lock()
	entries := b.buffer
	b.buffer = make(map[uint][]snapshotEntry)
	b.mu.Unlock()

	for _, list := range entries {
		for _, e := range list {
			db.Create(&models.TrafficSnapshot{
				TunnelID:  e.tunnelID,
				RxBytes:   e.rxBytes,
				TxBytes:   e.txBytes,
				LatencyMs: e.latencyMs,
				CreatedAt: time.Now(),
			})
		}
	}

	// Cleanup data older than 24h
	db.Where("created_at < ?", time.Now().Add(-24*time.Hour)).Delete(&models.TrafficSnapshot{})
}

var collector = &snapshotBatcher{
	buffer: make(map[uint][]snapshotEntry),
}

// StartCollector listens for Agent stats and writes to DB.
func StartCollector(db *gorm.DB, registry *ws.Registry) {
	registry.SetOnRecv(func(ac *ws.AgentConn, msg ws.Message) {
		if msg.Type != "stats" {
			return
		}

		var stats statsMessage
		if err := json.Unmarshal(msg.Data, &stats); err != nil {
			log.Printf("collector: invalid stats from node %d: %v", ac.NodeID, err)
			return
		}

		for tunnelIDStr, ts := range stats.Tunnels {
			var tunnelID uint
			fmt.Sscanf(tunnelIDStr, "tun-%d", &tunnelID)
			if tunnelID == 0 {
				continue
			}

			db.Table("tunnels").Where("id = ?", tunnelID).Updates(map[string]any{
				"rx_bytes":   gorm.Expr("rx_bytes + ?", ts.RxBytes),
				"tx_bytes":   gorm.Expr("tx_bytes + ?", ts.TxBytes),
				"latency_ms": ts.LatencyMs,
				"updated_at": time.Now(),
			})
		}

		db.Table("nodes").Where("id = ?", ac.NodeID).Updates(map[string]any{
			"cpu":       int(stats.System.CPUPercent),
			"memory_mb": stats.System.MemUsedMB,
			"last_seen": time.Now(),
		})

		collector.add(ac.NodeID, stats)
	})

	// Flush snapshots every 10 seconds
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			collector.flush(db)
		}
	}()
}
