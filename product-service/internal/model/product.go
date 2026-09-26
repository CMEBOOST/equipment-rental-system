package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Status values per CONTRACT.md §8.2/§11 (lowercase enum, "available"/"rented"
// plus "maintenance" — the third state is product-service's own addition for
// admin-managed downtime; it is not part of the rent/return flow rental-service
// drives).
const (
	StatusAvailable   = "available"
	StatusRented      = "rented"
	StatusMaintenance = "maintenance"
)

// ValidStatuses lists every value a status field may take. Kept as a slice
// (not a map) because its only consumer, IsValidStatus, is a short linear
// scan and a slice is what a validator "oneof" tag would enumerate too.
var ValidStatuses = []string{StatusAvailable, StatusRented, StatusMaintenance}

func IsValidStatus(s string) bool {
	for _, v := range ValidStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// ID deliberately carries no GORM "default:gen_random_uuid()" tag. The real
// (Postgres) schema comes entirely from the raw-SQL migration in
// migrations/000001_init_schema.up.sql, which already declares that
// default — GORM's AutoMigrate is never run against it in production. The
// tag would only matter for a driver that DOES go through AutoMigrate,
// which is exactly what the SQLite driver in tests does, and
// gen_random_uuid() does not exist there (it errors "syntax error" on the
// CREATE TABLE). So instead, every place that constructs a new Product
// (ProductService.Create, and test fixtures that insert one directly) sets
// ID explicitly with uuid.New(), matching user-service's convention
// throughout its service layer (see e.g.
// user-service/internal/service/user_service.go).
type Product struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	CategoryID  uuid.UUID
	Category    Category `gorm:"foreignKey:CategoryID"`
	Name        string   `gorm:"size:150"`
	Description string
	PricePerDay float64 `gorm:"column:price_per_day;type:decimal(10,2)"`
	Status      string  `gorm:"size:20;default:available"`
	ImageURL    string  `gorm:"column:image_url;size:255"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (Product) TableName() string { return "products" }
