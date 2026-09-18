package service_test

import (
	"fmt"
	"sync"
	"sync/atomic"
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
	// TranslateError is required so unique-constraint violations from Create()
	// surface as gorm.ErrDuplicatedKey (see AuthService.Register's TOCTOU guard)
	// instead of the raw, driver-specific SQLite error.
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
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

func TestAuthService_Register_TOCTOURace_UsernameViolation(t *testing.T) {
	svc, db := setupAuthServiceWithDB(t, ":memory:")
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"11111111-1111-1111-1111-111111111111", "other@example.com", "raceuser", "hash", "Other", 3, true,
	).Error)

	req := dto.RegisterRequest{Email: "newperson@example.com", Username: "raceuser", Password: "Passw0rd1"}
	_, err := svc.Register(req)
	require.ErrorIs(t, err, service.ErrUsernameExists)
}

var registerRaceTestDBCounter atomic.Int64

func TestAuthService_Register_ConcurrentSameEmail_OneSucceedsOtherGetsProperError(t *testing.T) {
	// Use a DSN unique to this specific test invocation so shared-cache in-memory
	// databases from other tests, or repeated invocations of this same test within
	// the same process (e.g. go test -count=2), can never collide with this one.
	dsn := fmt.Sprintf("file:register_race_test_%d?mode=memory&cache=shared", registerRaceTestDBCounter.Add(1))
	svc, _ := setupAuthServiceWithDB(t, dsn)
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

func TestAuthService_Login_WrongPassword_ReturnsErrInvalidCredentials(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "c@example.com", Username: "userc", Password: "Passw0rd1"})
	require.NoError(t, err)

	_, _, _, _, err = svc.Login(dto.LoginRequest{Email: "c@example.com", Password: "WrongPass1"}, "1.2.3.4", "test-agent")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestAuthService_Login_Success_ReturnsTokens(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "d@example.com", Username: "userd", Password: "Passw0rd1"})
	require.NoError(t, err)

	access, refresh, expiresIn, user, err := svc.Login(dto.LoginRequest{Email: "d@example.com", Password: "Passw0rd1"}, "1.2.3.4", "test-agent")
	require.NoError(t, err)
	require.NotEmpty(t, access)
	require.Len(t, refresh, 64)
	require.Equal(t, 900, expiresIn)
	require.Equal(t, "userd", user.Username)
}

func TestAuthService_Login_DisabledAccount_ReturnsErrAccountDisabled(t *testing.T) {
	svc := setupAuthService(t)
	u, err := svc.Register(dto.RegisterRequest{Email: "e@example.com", Username: "usere", Password: "Passw0rd1"})
	require.NoError(t, err)
	u.IsActive = false
	require.NoError(t, svc.UsersRepoForTest().Update(u))

	_, _, _, _, err = svc.Login(dto.LoginRequest{Email: "e@example.com", Password: "Passw0rd1"}, "1.2.3.4", "test-agent")
	require.ErrorIs(t, err, service.ErrAccountDisabled)
}

func TestAuthService_Login_NonexistentEmail_ReturnsErrInvalidCredentials(t *testing.T) {
	svc := setupAuthService(t)

	_, _, _, _, err := svc.Login(dto.LoginRequest{Email: "nobody@example.com", Password: "Passw0rd1"}, "1.2.3.4", "test-agent")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestAuthService_Login_WritesLoginLog_OnSuccessAndFailure(t *testing.T) {
	svc, db := setupAuthServiceWithDB(t, ":memory:")
	u, err := svc.Register(dto.RegisterRequest{Email: "f@example.com", Username: "userf", Password: "Passw0rd1"})
	require.NoError(t, err)

	_, _, _, _, err = svc.Login(dto.LoginRequest{Email: "f@example.com", Password: "WrongPass1"}, "9.9.9.9", "agent-fail")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)

	_, _, _, _, err = svc.Login(dto.LoginRequest{Email: "f@example.com", Password: "Passw0rd1"}, "9.9.9.9", "agent-success")
	require.NoError(t, err)

	var logs []model.LoginLog
	require.NoError(t, db.Where("email_attempted = ?", "f@example.com").Order("id asc").Find(&logs).Error)
	require.Len(t, logs, 2)

	require.False(t, logs[0].Success)
	require.NotNil(t, logs[0].UserID)
	require.Equal(t, u.ID, *logs[0].UserID)
	require.Equal(t, "agent-fail", logs[0].UserAgent)

	require.True(t, logs[1].Success)
	require.NotNil(t, logs[1].UserID)
	require.Equal(t, u.ID, *logs[1].UserID)
	require.Equal(t, "agent-success", logs[1].UserAgent)
}

func TestAuthService_Login_NonexistentEmail_WritesLoginLogWithNilUserID(t *testing.T) {
	svc, db := setupAuthServiceWithDB(t, ":memory:")

	_, _, _, _, err := svc.Login(dto.LoginRequest{Email: "ghost@example.com", Password: "whatever1"}, "1.1.1.1", "ghost-agent")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)

	var logs []model.LoginLog
	require.NoError(t, db.Where("email_attempted = ?", "ghost@example.com").Find(&logs).Error)
	require.Len(t, logs, 1)
	require.False(t, logs[0].Success)
	require.Nil(t, logs[0].UserID)
}

func TestAuthService_Refresh_RotatesToken(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "f@example.com", Username: "userf", Password: "Passw0rd1"})
	require.NoError(t, err)
	_, refresh, _, _, err := svc.Login(dto.LoginRequest{Email: "f@example.com", Password: "Passw0rd1"}, "ip", "ua")
	require.NoError(t, err)

	_, newRefresh, expiresIn, err := svc.Refresh(refresh)
	require.NoError(t, err)
	require.NotEqual(t, refresh, newRefresh)
	require.Equal(t, 900, expiresIn)

	// old token must now be rejected
	_, _, _, err = svc.Refresh(refresh)
	require.ErrorIs(t, err, service.ErrInvalidRefreshToken)
}

func TestAuthService_Logout_RevokesToken(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "g@example.com", Username: "userg", Password: "Passw0rd1"})
	require.NoError(t, err)
	_, refresh, _, _, err := svc.Login(dto.LoginRequest{Email: "g@example.com", Password: "Passw0rd1"}, "ip", "ua")
	require.NoError(t, err)

	require.NoError(t, svc.Logout(refresh))
	_, _, _, err = svc.Refresh(refresh)
	require.ErrorIs(t, err, service.ErrInvalidRefreshToken)
}
