package model

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID         uuid.UUID `gorm:"primaryKey;default:gen_random_uuid()"`
	UserID     uuid.UUID
	TokenHash  string `gorm:"unique;size:64"`
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	IPAddress  string `gorm:"size:45"`
	UserAgent  string `gorm:"size:255"`
	CreatedAt  time.Time
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
