package models

import "time"

type Tunnel struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;not null" json:"name"`
	LeftNodeID  uint      `gorm:"index;not null" json:"left_node_id"`
	RightNodeID uint      `gorm:"index;not null" json:"right_node_id"`
	LeftSubnet  string    `gorm:"size:100" json:"left_subnet"`
	RightSubnet string    `gorm:"size:100" json:"right_subnet"`
	Status      string    `gorm:"size:20;default:down" json:"status"`
	RxBytes     int64     `gorm:"default:0" json:"rx_bytes"`
	TxBytes     int64     `gorm:"default:0" json:"tx_bytes"`
	LatencyMs   float64   `gorm:"default:0" json:"latency_ms"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	LeftNode  Node `gorm:"foreignKey:LeftNodeID" json:"left_node,omitempty"`
	RightNode Node `gorm:"foreignKey:RightNodeID" json:"right_node,omitempty"`
}
