package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

var ErrMissingInternalAPIKey = errors.New("INTERNAL_API_KEY is required")

type Config struct {
	AppPort                                    string
	DBHost, DBPort, DBUser, DBPassword, DBName string
	InternalAPIKey                             string
	ProductServiceURL                          string
	UserServiceURL                             string
	CORSOrigin                                 string
	MigrationsPath                             string
}

func Load() (*Config, error) {
	_ = godotenv.Load()
	key := getEnv("INTERNAL_API_KEY", "")
	if key == "" {
		return nil, ErrMissingInternalAPIKey
	}
	return &Config{
		AppPort:           getEnv("APP_PORT", "8083"),
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "5432"),
		DBUser:            getEnv("DB_USER", "rental_service"),
		DBPassword:        getEnv("DB_PASSWORD", "secret"),
		DBName:            getEnv("DB_NAME", "rental_db"),
		InternalAPIKey:    key,
		ProductServiceURL: getEnv("PRODUCT_SERVICE_URL", "http://product-service:8082/api/v1"),
		UserServiceURL:    getEnv("USER_SERVICE_URL", "http://user-service:8081/api/v1"),
		CORSOrigin:        getEnv("CORS_ORIGIN", "http://localhost:3000"),
		MigrationsPath:    getEnv("MIGRATIONS_PATH", "migrations"),
	}, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
