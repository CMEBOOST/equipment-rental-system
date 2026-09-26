package db

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/config"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	// TranslateError surfaces unique-constraint violations as the
	// driver-agnostic gorm.ErrDuplicatedKey (used for the category-name
	// uniqueness check) instead of a raw, driver-specific error.
	return gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
}
