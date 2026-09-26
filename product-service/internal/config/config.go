package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// DefaultCORSOrigin mirrors user-service's default: the frontend's dev
// server origin, used when CORS_ORIGIN is not set.
const DefaultCORSOrigin = "http://localhost:3000"

// ErrMissingInternalAPIKey is returned by Load when INTERNAL_API_KEY is
// unset. product-service does not hold JWT_SECRET (Kong verifies token
// signatures — CONTRACT.md §5.3), but it still needs INTERNAL_API_KEY to
// protect the internal-only status-update path rental-service calls
// directly (bypassing Kong).
var ErrMissingInternalAPIKey = errors.New("INTERNAL_API_KEY is required")

type Config struct {
	AppPort                                    string
	DBHost, DBPort, DBUser, DBPassword, DBName string
	InternalAPIKey                             string
	CORSOrigin                                 string
	// MigrationsPath is the directory holding the .sql migration files that
	// db.Migrate applies at startup — see user-service/internal/config for
	// why this is applied at startup rather than as a separate manual step.
	MigrationsPath string
}

// Load reads configuration from the environment (plus an optional .env
// file) and validates INTERNAL_API_KEY at startup rather than leaving an
// empty value to fail open at request time (see
// middleware.RequireInternalKey: an empty expected key would match a
// request that omits the header entirely).
func Load() (*Config, error) {
	_ = godotenv.Load() // ignore error: absent in prod containers, present in dev

	internalAPIKey := getEnv("INTERNAL_API_KEY", "")
	if internalAPIKey == "" {
		return nil, ErrMissingInternalAPIKey
	}

	return &Config{
		AppPort:        getEnv("APP_PORT", "8082"),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "product_service"),
		DBPassword:     getEnv("DB_PASSWORD", "secret"),
		DBName:         getEnv("DB_NAME", "product_db"),
		InternalAPIKey: internalAPIKey,
		CORSOrigin:     getEnv("CORS_ORIGIN", DefaultCORSOrigin),
		MigrationsPath: getEnv("MIGRATIONS_PATH", "migrations"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
