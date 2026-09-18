package repository_test

import (
	"testing"

	"github.com/glebarez/sqlite" // pure-Go sqlite driver, no CGO needed
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
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
	return db
}

func TestUserRepo_CreateAndFindByEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewUserRepo(db)

	userID := uuid.New()
	u := &model.User{ID: userID, Email: "a@example.com", Username: "a", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(u))

	found, err := repo.FindByEmail("a@example.com")
	require.NoError(t, err)
	require.Equal(t, userID, found.ID)
}
