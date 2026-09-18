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

func TestUserRepo_List_DefaultsPagination(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewUserRepo(db)

	// Create test users
	for i := 0; i < 5; i++ {
		u := &model.User{
			ID:           uuid.New(),
			Email:        "user" + string(rune('0'+i)) + "@example.com",
			Username:     "user" + string(rune('0'+i)),
			PasswordHash: "hash",
			RoleID:       3,
		}
		require.NoError(t, repo.Create(u))
	}

	// Test with page=0 (should default to 1)
	users, total, err := repo.List(repository.UserFilter{Page: 0, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, users, 5)

	// Test with limit=0 (should default to 20)
	users, total, err = repo.List(repository.UserFilter{Page: 1, Limit: 0})
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, users, 5)

	// Test with limit > 100 (should default to 20)
	users, total, err = repo.List(repository.UserFilter{Page: 1, Limit: 150})
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, users, 5)

	// Test with limit=-5 (should default to 20)
	users, total, err = repo.List(repository.UserFilter{Page: 1, Limit: -5})
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, users, 5)

	// Test with negative page (should default to 1)
	users, total, err = repo.List(repository.UserFilter{Page: -10, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, users, 5)
}

func TestUserRepo_List_DefaultsSort(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewUserRepo(db)

	// Create test users with different timestamps
	user1 := &model.User{ID: uuid.New(), Email: "a@example.com", Username: "a", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(user1))

	user2 := &model.User{ID: uuid.New(), Email: "b@example.com", Username: "b", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(user2))

	// Test with empty Sort and Order (should default to created_at, desc)
	users, _, err := repo.List(repository.UserFilter{Page: 1, Limit: 10, Sort: "", Order: ""})
	require.NoError(t, err)
	require.Len(t, users, 2)
	// Most recently created should come first (desc)
	require.Equal(t, user2.ID, users[0].ID)
	require.Equal(t, user1.ID, users[1].ID)
}

func TestUserRepo_List_ValidSort(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewUserRepo(db)

	user1 := &model.User{ID: uuid.New(), Email: "a@example.com", Username: "a", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(user1))

	user2 := &model.User{ID: uuid.New(), Email: "b@example.com", Username: "b", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(user2))

	// Test with valid sort column (email)
	users, _, err := repo.List(repository.UserFilter{Page: 1, Limit: 10, Sort: "email", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, users, 2)
	// Should be sorted by email ascending
	require.Equal(t, "a@example.com", users[0].Email)
	require.Equal(t, "b@example.com", users[1].Email)
}

func TestUserRepo_List_InvalidSortFallsBackToDefault(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewUserRepo(db)

	user1 := &model.User{ID: uuid.New(), Email: "a@example.com", Username: "a", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(user1))

	user2 := &model.User{ID: uuid.New(), Email: "b@example.com", Username: "b", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(user2))

	// Test with invalid sort column (should not crash and should use default created_at)
	users, _, err := repo.List(repository.UserFilter{Page: 1, Limit: 10, Sort: "'; DROP TABLE users; --", Order: "desc"})
	require.NoError(t, err)
	require.Len(t, users, 2)
	// Should complete successfully without SQL injection
}

func TestUserRepo_List_InvalidOrderFallsBackToDesc(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewUserRepo(db)

	user1 := &model.User{ID: uuid.New(), Email: "a@example.com", Username: "a", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(user1))

	user2 := &model.User{ID: uuid.New(), Email: "b@example.com", Username: "b", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(user2))

	// Test with invalid order (should fallback to desc)
	users, _, err := repo.List(repository.UserFilter{Page: 1, Limit: 10, Sort: "email", Order: "INVALID"})
	require.NoError(t, err)
	require.Len(t, users, 2)
	// Most recent should come first when defaulted to desc
	require.Equal(t, user2.ID, users[0].ID)
	require.Equal(t, user1.ID, users[1].ID)

	// Test with order=asc (should work)
	users, _, err = repo.List(repository.UserFilter{Page: 1, Limit: 10, Sort: "email", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, users, 2)
	require.Equal(t, "a@example.com", users[0].Email)
	require.Equal(t, "b@example.com", users[1].Email)

	// Test with order=ASC (uppercase, should work - case insensitive)
	users, _, err = repo.List(repository.UserFilter{Page: 1, Limit: 10, Sort: "email", Order: "ASC"})
	require.NoError(t, err)
	require.Len(t, users, 2)
	require.Equal(t, "a@example.com", users[0].Email)
	require.Equal(t, "b@example.com", users[1].Email)
}
