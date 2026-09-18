package service_test

import (
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func setupAuthServiceWithDB(t *testing.T, dsn string) (*service.AuthService, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	// Create tables manually for SQLite (gen_random_uuid() is not supported)
	require.NoError(t, db.Exec(`
		CREATE TABLE roles (
			id INTEGER PRIMARY KEY,
			name TEXT UNIQUE,
			description TEXT
		)
	`).Error)

	require.NoError(t, db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE,
			username TEXT UNIQUE,
			password_hash TEXT,
			full_name TEXT,
			phone TEXT,
			role_id INTEGER,
			is_active BOOLEAN DEFAULT true,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			FOREIGN KEY (role_id) REFERENCES roles(id)
		)
	`).Error)

	require.NoError(t, db.Exec(`
		CREATE TABLE refresh_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			token_hash TEXT UNIQUE,
			expires_at DATETIME,
			revoked_at DATETIME,
			ip_address TEXT,
			user_agent TEXT,
			created_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`).Error)

	require.NoError(t, db.Exec(`
		CREATE TABLE login_logs (
			id INTEGER PRIMARY KEY,
			user_id TEXT,
			email_attempted TEXT,
			success BOOLEAN,
			ip_address TEXT,
			user_agent TEXT,
			created_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`).Error)

	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)

	cfg := &config.Config{BCryptCost: 4, JWTSecret: "test-secret-min-32-characters-ok", JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 168 * time.Hour}
	return service.NewAuthService(
		repository.NewUserRepo(db), repository.NewRoleRepo(db),
		repository.NewRefreshTokenRepo(db), repository.NewLoginLogRepo(db),
		service.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL), cfg,
	), db
}

func setupAuthService(t *testing.T) *service.AuthService {
	svc, _ := setupAuthServiceWithDB(t, ":memory:")
	return svc
}

func TestAuthService_Register_DuplicateEmail_ReturnsErrEmailExists(t *testing.T) {
	svc := setupAuthService(t)
	req := dto.RegisterRequest{Email: "a@example.com", Username: "user1", Password: "Passw0rd1", FullName: "A"}

	_, err := svc.Register(req)
	require.NoError(t, err)

	req.Username = "user2"
	_, err = svc.Register(req)
	require.ErrorIs(t, err, service.ErrEmailExists)
}

func TestAuthService_Register_AssignsCustomerRole(t *testing.T) {
	svc := setupAuthService(t)
	u, err := svc.Register(dto.RegisterRequest{Email: "b@example.com", Username: "userb", Password: "Passw0rd1"})
	require.NoError(t, err)
	require.Equal(t, int16(3), u.RoleID)
}

func TestAuthService_Register_ConcurrentSameEmail_OneSucceedsOtherGetsProperError(t *testing.T) {
	// Use shared-cache in-memory database to allow concurrent access from goroutines
	svc, _ := setupAuthServiceWithDB(t, "file::memory:?cache=shared")
	req1 := dto.RegisterRequest{Email: "race@example.com", Username: "racer1", Password: "Passw0rd1"}
	req2 := dto.RegisterRequest{Email: "race@example.com", Username: "racer2", Password: "Passw0rd1"}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); _, errs[0] = svc.Register(req1) }()
	go func() { defer wg.Done(); _, errs[1] = svc.Register(req2) }()
	wg.Wait()

	successCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
		} else {
			require.ErrorIs(t, err, service.ErrEmailExists)
		}
	}
	require.Equal(t, 1, successCount)
}
