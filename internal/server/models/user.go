package models

import "time"

type User struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Username       string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash   string    `gorm:"size:255;not null" json:"-"`
	MustChangePass bool      `gorm:"default:true" json:"must_change_pass"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
