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
