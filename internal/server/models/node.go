package models

import "time"

type Node struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:100;not null" json:"name"`
	Host         string    `gorm:"size:255" json:"-"`
	VirtualIP    string    `gorm:"size:45" json:"virtual_ip"`
	Subnets      string    `gorm:"text" json:"subnets"`       // advertised subnets (unique, conflict-free)
	LocalSubnets string    `gorm:"text" json:"local_subnets"` // raw detected subnets (may conflict)
	AgentToken   string    `gorm:"size:64;uniqueIndex" json:"-"`
	AgentVersion string    `gorm:"size:20" json:"agent_version"`
	Connected    bool      `json:"connected"`
	Backbone     bool      `json:"backbone"`
	ListenAddr   string    `gorm:"size:100" json:"listen_addr"`
	CPU          int       `gorm:"default:0" json:"cpu"`
	MemoryMB     int       `gorm:"default:0" json:"memory_mb"`
	OSInfo       string    `gorm:"size:200" json:"os_info"`
	LastSeen     time.Time `json:"last_seen"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
