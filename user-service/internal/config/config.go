package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort        string
	DBHost, DBPort, DBUser, DBPassword, DBName string
	JWTSecret      string
	JWTAccessTTL   time.Duration
	JWTRefreshTTL  time.Duration
	InternalAPIKey string
	BCryptCost     int
}

func Load() (*Config, error) {
	_ = godotenv.Load() // ignore error: absent in prod containers, present in dev
	accessTTL, err := time.ParseDuration(getEnv("JWT_ACCESS_TTL", "15m"))
	if err != nil {
		return nil, err
	}
	refreshTTL, err := time.ParseDuration(getEnv("JWT_REFRESH_TTL", "168h"))
	if err != nil {
		return nil, err
	}
	cost, err := strconv.Atoi(getEnv("BCRYPT_COST", "12"))
	if err != nil {
		return nil, err
	}
	return &Config{
		AppPort:        getEnv("APP_PORT", "8081"),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "user_service"),
		DBPassword:     getEnv("DB_PASSWORD", "secret"),
		DBName:         getEnv("DB_NAME", "user_db"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		JWTAccessTTL:   accessTTL,
		JWTRefreshTTL:  refreshTTL,
		InternalAPIKey: getEnv("INTERNAL_API_KEY", ""),
		BCryptCost:     cost,
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
