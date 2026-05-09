package models

import "time"

type TrafficSnapshot struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TunnelID  uint      `gorm:"index" json:"tunnel_id"`
	RxBytes   int64     `json:"rx_bytes"`
	TxBytes   int64     `json:"tx_bytes"`
	LatencyMs float64   `json:"latency_ms"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
