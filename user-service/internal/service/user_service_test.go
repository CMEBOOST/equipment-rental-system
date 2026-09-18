package service_test

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

var testCfg = &config.Config{BCryptCost: 4}

func setupUserService(t *testing.T) (*service.UserService, *model.User) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Create tables manually for SQLite
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

	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)

	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID.String(), "p@example.com", "p", "x", "Old Name", 3, true,
	).Error)

	u := &model.User{
		ID:           userID,
		Email:        "p@example.com",
		Username:     "p",
		PasswordHash: "x",
		FullName:     "Old Name",
		RoleID:       3,
		IsActive:     true,
	}
	return service.NewUserService(repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db), testCfg, repository.NewLoginLogRepo(db)), u
}

func setupUserServiceWithPassword(t *testing.T, plain string) (*service.UserService, *model.User, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

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

	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)

	hash, err := bcrypt.GenerateFromPassword([]byte(plain), 4)
	require.NoError(t, err)
	u := &model.User{ID: uuid.New(), Email: "q@example.com", Username: "q", PasswordHash: string(hash), RoleID: 3}
	require.NoError(t, db.Create(u).Error)

	return service.NewUserService(repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db), testCfg, repository.NewLoginLogRepo(db)), u, db
}

func TestUserService_ChangePassword_WrongCurrent_ReturnsError(t *testing.T) {
	svc, u, _ := setupUserServiceWithPassword(t, "Passw0rd1")
	err := svc.ChangePassword(u.ID, dto.ChangePasswordRequest{CurrentPassword: "WrongOne1", NewPassword: "N3wPassw0rd"})
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestUserService_ChangePassword_WeakNewPassword_AllDigits_ReturnsError(t *testing.T) {
	svc, u, _ := setupUserServiceWithPassword(t, "Passw0rd1")
	err := svc.ChangePassword(u.ID, dto.ChangePasswordRequest{CurrentPassword: "Passw0rd1", NewPassword: "12345678"})
	require.ErrorIs(t, err, service.ErrWeakPassword)
}

func TestUserService_ChangePassword_WeakNewPassword_AllLetters_ReturnsError(t *testing.T) {
	svc, u, _ := setupUserServiceWithPassword(t, "Passw0rd1")
	err := svc.ChangePassword(u.ID, dto.ChangePasswordRequest{CurrentPassword: "Passw0rd1", NewPassword: "abcdefgh"})
	require.ErrorIs(t, err, service.ErrWeakPassword)
}

func TestUserService_ChangePassword_WeakNewPassword_DoesNotChangeHashOrRevokeSessions(t *testing.T) {
	svc, u, db := setupUserServiceWithPassword(t, "Passw0rd1")
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: u.ID, TokenHash: "hash-1", ExpiresAt: time.Now().Add(time.Hour),
	}))

	err := svc.ChangePassword(u.ID, dto.ChangePasswordRequest{CurrentPassword: "Passw0rd1", NewPassword: "12345678"})
	require.ErrorIs(t, err, service.ErrWeakPassword)

	userRepo := repository.NewUserRepo(db)
	reloaded, findErr := userRepo.FindByID(u.ID)
	require.NoError(t, findErr)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte("Passw0rd1")))

	active, listErr := refreshRepo.ListActiveForUser(u.ID)
	require.NoError(t, listErr)
	require.Len(t, active, 1)
}

func TestUserService_ChangePassword_WrongCurrent_DoesNotChangeHashOrRevokeSessions(t *testing.T) {
	svc, u, db := setupUserServiceWithPassword(t, "Passw0rd1")
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: u.ID, TokenHash: "hash-1", ExpiresAt: time.Now().Add(time.Hour),
	}))

	err := svc.ChangePassword(u.ID, dto.ChangePasswordRequest{CurrentPassword: "WrongOne1", NewPassword: "N3wPassw0rd"})
	require.ErrorIs(t, err, service.ErrInvalidCredentials)

	// password hash must be untouched
	userRepo := repository.NewUserRepo(db)
	reloaded, findErr := userRepo.FindByID(u.ID)
	require.NoError(t, findErr)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte("Passw0rd1")))

	// session must remain active (not revoked)
	active, listErr := refreshRepo.ListActiveForUser(u.ID)
	require.NoError(t, listErr)
	require.Len(t, active, 1)
}

func TestUserService_ChangePassword_Correct_RevokesAllSessions(t *testing.T) {
	svc, u, db := setupUserServiceWithPassword(t, "Passw0rd1")
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: u.ID, TokenHash: "hash-1", ExpiresAt: time.Now().Add(time.Hour),
	}))
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: u.ID, TokenHash: "hash-2", ExpiresAt: time.Now().Add(time.Hour),
	}))
	active, err := refreshRepo.ListActiveForUser(u.ID)
	require.NoError(t, err)
	require.Len(t, active, 2, "sanity check: both sessions active before password change")

	err = svc.ChangePassword(u.ID, dto.ChangePasswordRequest{CurrentPassword: "Passw0rd1", NewPassword: "N3wPassw0rd"})
	require.NoError(t, err)

	// all sessions (not just one) must now be revoked
	active, err = refreshRepo.ListActiveForUser(u.ID)
	require.NoError(t, err)
	require.Empty(t, active)

	// new password hash must actually be stored
	userRepo := repository.NewUserRepo(db)
	reloaded, findErr := userRepo.FindByID(u.ID)
	require.NoError(t, findErr)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte("N3wPassw0rd")))
	require.Error(t, bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte("Passw0rd1")))
}

func TestUserService_UpdateProfile_ChangesFullNameAndPhone(t *testing.T) {
	svc, u := setupUserService(t)
	newName := "New Name"
	updated, err := svc.UpdateProfile(u.ID, dto.UpdateProfileRequest{FullName: &newName})
	require.NoError(t, err)
	require.Equal(t, "New Name", updated.FullName)
	require.Equal(t, u.Email, updated.Email) // email untouched
}

func TestUserService_UpdateProfile_UpdatesPhone(t *testing.T) {
	svc, u := setupUserService(t)
	newPhone := "+66812345678"
	updated, err := svc.UpdateProfile(u.ID, dto.UpdateProfileRequest{Phone: &newPhone})
	require.NoError(t, err)
	require.Equal(t, newPhone, updated.Phone)
	require.Equal(t, "Old Name", updated.FullName) // full_name untouched
	require.Equal(t, u.Email, updated.Email)       // email untouched
}

func TestUserService_UpdateProfile_UpdatesBothFullNameAndPhone(t *testing.T) {
	svc, u := setupUserService(t)
	newName := "Updated Name"
	newPhone := "+66887654321"
	updated, err := svc.UpdateProfile(u.ID, dto.UpdateProfileRequest{FullName: &newName, Phone: &newPhone})
	require.NoError(t, err)
	require.Equal(t, newName, updated.FullName)
	require.Equal(t, newPhone, updated.Phone)
	require.Equal(t, u.Email, updated.Email) // email untouched
}

func TestUserService_UpdateProfile_IgnoresExtraFieldsInRequest(t *testing.T) {
	svc, u := setupUserService(t)
	newName := "New Name"
	// Simulate request with extra fields that shouldn't be processed
	// The UpdateProfileRequest only has FullName and Phone pointers,
	// so any other fields in JSON won't be deserialized or applied
	updated, err := svc.UpdateProfile(u.ID, dto.UpdateProfileRequest{
		FullName: &newName,
		Phone:    nil,
		// Extra fields like Email, Username, RoleID are not in the struct
		// so they cannot be passed - this is a compile-time guarantee
	})
	require.NoError(t, err)
	require.Equal(t, newName, updated.FullName)
	require.Equal(t, u.Email, updated.Email)       // email untouched
	require.Equal(t, u.Username, updated.Username) // username untouched
	require.Equal(t, u.RoleID, updated.RoleID)     // role untouched
}
