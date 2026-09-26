package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending   = "pending"
	StatusActive    = "active"
	StatusReturned  = "returned"
	StatusCancelled = "cancelled"
)

type Rental struct {
	ID         uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	ProductID  uuid.UUID  `gorm:"type:uuid;not null;index"`
	StartDate  time.Time  `gorm:"type:date;not null"`
	DueDate    time.Time  `gorm:"type:date;not null"`
	ReturnDate *time.Time `gorm:"type:date"`
	TotalPrice float64    `gorm:"type:numeric(12,2);not null"`
	Status     string     `gorm:"size:20;not null;index"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (Rental) TableName() string { return "rentals" }
