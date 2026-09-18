package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID `gorm:"primaryKey;type:text"`
	Email        string    `gorm:"unique;size:255"`
	Username     string    `gorm:"unique;size:50"`
	PasswordHash string    `gorm:"size:255"`
	FullName     string    `gorm:"size:120"`
	Phone        string    `gorm:"size:20"`
	RoleID       int16
	Role         Role `gorm:"foreignKey:RoleID"`
	IsActive     bool `gorm:"default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (User) TableName() string { return "users" }
