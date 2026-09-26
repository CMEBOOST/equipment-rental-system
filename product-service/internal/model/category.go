package model

import (
	"time"

	"github.com/google/uuid"
)

// ID has no GORM "default:gen_random_uuid()" tag: production schema comes
// from the raw-SQL migration (which does declare that default), never from
// GORM AutoMigrate, and the function does not exist under the SQLite driver
// tests use — see model/product.go's ID field for the full reasoning. Code
// that creates a Category must set ID explicitly (uuid.New()), matching
// user-service's convention throughout its service layer.
type Category struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	Name        string    `gorm:"unique;size:100"`
	Description string    `gorm:"size:255"`
	CreatedAt   time.Time
}

func (Category) TableName() string { return "categories" }
