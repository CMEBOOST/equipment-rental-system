package service_test

import (
	"fmt"
	"sync"
	"sync/atomic"
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

// setupUserServiceForCreate builds a UserService plus RoleRepo backed by a
// SQLite DB opened with TranslateError: true, which is required for
// CreateUser's TOCTOU guard to see gorm.ErrDuplicatedKey (rather than a raw,
// driver-specific SQLite error) when Create() hits a unique constraint.
func setupUserServiceForCreate(t *testing.T, dsn string) (*service.UserService, *repository.RoleRepo, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
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

	require.NoError(t, db.Create(&model.Role{ID: 2, Name: "staff"}).Error)

	return service.NewUserService(repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db), testCfg, repository.NewLoginLogRepo(db)),
		repository.NewRoleRepo(db), db
}

func TestUserService_CreateUser_AssignsRequestedRole(t *testing.T) {
	svc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")

	u, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)
	require.Equal(t, int16(2), u.RoleID)
}

func TestUserService_CreateUser_GeneratesUniqueIDsAcrossMultipleUsers(t *testing.T) {
	// Guards against a real bug class: if CreateUser forgets to set a fresh
	// uuid.New() on the model before Create(), every admin-created user would
	// share the zero UUID (SQLite test tables have no UUID-generating default),
	// so the second create would collide on the primary key.
	svc, roleRepo, db := setupUserServiceForCreate(t, ":memory:")

	u1, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)

	u2, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff2@example.com", Username: "staff2", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)

	require.NotEqual(t, uuid.UUID{}, u1.ID, "created user must not have the zero UUID")
	require.NotEqual(t, uuid.UUID{}, u2.ID, "created user must not have the zero UUID")
	require.NotEqual(t, u1.ID, u2.ID)

	userRepo := repository.NewUserRepo(db)
	_, err = userRepo.FindByID(u1.ID)
	require.NoError(t, err)
	_, err = userRepo.FindByID(u2.ID)
	require.NoError(t, err)
}

func TestUserService_CreateUser_WeakPassword_ReturnsErrWeakPassword(t *testing.T) {
	svc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")

	_, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "allletters", Role: "staff",
	}, roleRepo, 4)
	require.ErrorIs(t, err, service.ErrWeakPassword)
}

func TestUserService_CreateUser_WeakPassword_DoesNotCreateUser(t *testing.T) {
	svc, roleRepo, db := setupUserServiceForCreate(t, ":memory:")

	_, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "12345678", Role: "staff",
	}, roleRepo, 4)
	require.ErrorIs(t, err, service.ErrWeakPassword)

	_, findErr := repository.NewUserRepo(db).FindByEmail("staff1@example.com")
	require.ErrorIs(t, findErr, gorm.ErrRecordNotFound, "a weak-password request must not persist a user row")
}

func TestUserService_CreateUser_DuplicateEmail_ReturnsErrEmailExists(t *testing.T) {
	svc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")

	_, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "dup@example.com", Username: "user1", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)

	_, err = svc.CreateUser(dto.CreateUserRequest{
		Email: "dup@example.com", Username: "user2", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.ErrorIs(t, err, service.ErrEmailExists)
}

func TestUserService_CreateUser_DuplicateUsername_ReturnsErrUsernameExists(t *testing.T) {
	svc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")

	_, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "user1@example.com", Username: "dupuser", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)

	_, err = svc.CreateUser(dto.CreateUserRequest{
		Email: "user2@example.com", Username: "dupuser", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.ErrorIs(t, err, service.ErrUsernameExists)
}

func TestUserService_CreateUser_UsesGivenCost_NotHardcoded(t *testing.T) {
	svc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")

	const wantCost = 5 // deliberately different from testCfg.BCryptCost (4)
	u, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, wantCost)
	require.NoError(t, err)

	gotCost, err := bcrypt.Cost([]byte(u.PasswordHash))
	require.NoError(t, err)
	require.Equal(t, wantCost, gotCost, "CreateUser must hash with the cost argument it was given, not a hardcoded value")
}

func TestUserService_CreateUser_IsActiveDefaultsToTrue(t *testing.T) {
	svc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")

	u, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)
	require.True(t, u.IsActive)
}

func TestUserService_CreateUser_IsActiveFalse_Honored(t *testing.T) {
	svc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")

	inactive := false
	u, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "St@ffPass1", Role: "staff", IsActive: &inactive,
	}, roleRepo, 4)
	require.NoError(t, err)
	require.False(t, u.IsActive)
}

var createUserRaceTestDBCounter atomic.Int64

func TestUserService_CreateUser_TOCTOURace_ConcurrentSameEmail_OneSucceedsOtherGetsProperError(t *testing.T) {
	dsn := fmt.Sprintf("file:create_user_race_test_%d?mode=memory&cache=shared", createUserRaceTestDBCounter.Add(1))
	svc, roleRepo, _ := setupUserServiceForCreate(t, dsn)

	req1 := dto.CreateUserRequest{Email: "race@example.com", Username: "racer1", Password: "St@ffPass1", Role: "staff"}
	req2 := dto.CreateUserRequest{Email: "race@example.com", Username: "racer2", Password: "St@ffPass1", Role: "staff"}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); _, errs[0] = svc.CreateUser(req1, roleRepo, 4) }()
	go func() { defer wg.Done(); _, errs[1] = svc.CreateUser(req2, roleRepo, 4) }()
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

func TestUserService_ListUsers_DelegatesToRepoWithFilter(t *testing.T) {
	svc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")

	_, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)
	_, err = svc.CreateUser(dto.CreateUserRequest{
		Email: "staff2@example.com", Username: "staff2", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)

	users, total, err := svc.ListUsers(repository.UserFilter{Role: "staff", Page: 1, Limit: 20})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, users, 2)
}
