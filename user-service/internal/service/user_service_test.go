package service_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

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
	return service.NewUserService(repository.NewUserRepo(db)), u
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
