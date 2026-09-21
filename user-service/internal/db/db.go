package db

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	// TranslateError is required so unique-constraint violations surface as the
	// driver-agnostic gorm.ErrDuplicatedKey (used by AuthService.Register's TOCTOU
	// race handling) instead of raw, driver-specific errors.
	return gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
}
