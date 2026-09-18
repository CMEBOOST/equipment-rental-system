package model

import (
	"time"

	"github.com/google/uuid"
)

type LoginLog struct {
	ID              int64 `gorm:"primaryKey"`
	UserID          *uuid.UUID
	EmailAttempted  string `gorm:"size:255"`
	Success         bool
	IPAddress       string `gorm:"size:45"`
	UserAgent       string `gorm:"size:255"`
	CreatedAt       time.Time
}

func (LoginLog) TableName() string { return "login_logs" }
