package models

import "time"

type AuditLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Action     string    `gorm:"size:50;not null;index" json:"action"`
	TargetType string    `gorm:"size:20;not null;index" json:"target_type"`
	TargetID   uint      `gorm:"index" json:"target_id"`
	Detail     string    `gorm:"text" json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}
