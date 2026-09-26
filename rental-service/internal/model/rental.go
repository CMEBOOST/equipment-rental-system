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
	ID         uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	ProductID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"product_id"`
	StartDate  time.Time  `gorm:"type:date;not null" json:"start_date"`
	DueDate    time.Time  `gorm:"type:date;not null" json:"due_date"`
	ReturnDate *time.Time `gorm:"type:date" json:"return_date,omitempty"`
	TotalPrice float64    `gorm:"type:numeric(12,2);not null" json:"total_price"`
	Status     string     `gorm:"size:20;not null;index" json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (Rental) TableName() string { return "rentals" }
