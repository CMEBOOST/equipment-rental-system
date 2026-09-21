package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// MinJWTSecretLen is the shortest JWT signing secret this service will start
// with. Design doc §8.1 requires at least 32 characters for the HS256 secret.
const MinJWTSecretLen = 32

// DefaultCORSOrigin is used when CORS_ORIGIN is not set: the origin the
// project's React frontend runs on in development. It is defined here rather
// than in the middleware package because config must not import middleware
// (middleware -> service -> config would be an import cycle).
const DefaultCORSOrigin = "http://localhost:3000"

// ErrMissingJWTSecret / ErrWeakJWTSecret / ErrMissingInternalAPIKey are the
// startup-time validation failures returned by Load. They are sentinels so
// tests (and any future callers) can assert on the specific misconfiguration
// instead of matching error strings.
var (
	ErrMissingJWTSecret      = errors.New("JWT_SECRET is required")
	ErrWeakJWTSecret         = fmt.Errorf("JWT_SECRET must be at least %d characters", MinJWTSecretLen)
	ErrMissingInternalAPIKey = errors.New("INTERNAL_API_KEY is required")
)

type Config struct {
	AppPort                                    string
	DBHost, DBPort, DBUser, DBPassword, DBName string
	JWTSecret                                  string
	JWTAccessTTL                               time.Duration
	JWTRefreshTTL                              time.Duration
	InternalAPIKey                             string
	BCryptCost                                 int
	// CORSOrigin is the single browser origin allowed to call this service
	// (design doc §4/§8.6). It defaults to the frontend's dev server.
	CORSOrigin string
	// MigrationsPath is the directory holding the .sql migration files that
	// db.Migrate applies at startup. The default is relative, which resolves
	// to ./migrations when running from the user-service directory; the
	// Docker image sets MIGRATIONS_PATH=/migrations, where the Dockerfile
	// copies them.
	MigrationsPath string
}

// Load reads configuration from the environment (plus an optional .env file)
// and validates the security-critical secrets.
//
// The two secrets are validated here, at startup, rather than being left to
// fail open at request time: an empty JWT_SECRET would make TokenService sign
// and verify with an empty HMAC key (so anyone can forge an access token),
// and an empty INTERNAL_API_KEY would make RequireInternalKey's comparison
// succeed for a request that simply omits the X-Internal-Key header. Both are
// silent, exploitable defaults, so an unconfigured deployment must refuse to
// start instead of starting insecurely (cmd/api/main.go log.Fatalf's on any
// error returned from here).
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
	jwtSecret := getEnv("JWT_SECRET", "")
	switch {
	case jwtSecret == "":
		return nil, ErrMissingJWTSecret
	case len(jwtSecret) < MinJWTSecretLen:
		return nil, ErrWeakJWTSecret
	}
	internalAPIKey := getEnv("INTERNAL_API_KEY", "")
	if internalAPIKey == "" {
		return nil, ErrMissingInternalAPIKey
	}
	return &Config{
		AppPort:        getEnv("APP_PORT", "8081"),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "user_service"),
		DBPassword:     getEnv("DB_PASSWORD", "secret"),
		DBName:         getEnv("DB_NAME", "user_db"),
		JWTSecret:      jwtSecret,
		JWTAccessTTL:   accessTTL,
		JWTRefreshTTL:  refreshTTL,
		InternalAPIKey: internalAPIKey,
		BCryptCost:     cost,
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
