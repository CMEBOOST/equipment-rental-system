package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/handler"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func setupUserHandler(t *testing.T) (*handler.UserHandler, *model.User, *gorm.DB) {
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
		`INSERT INTO users (id, email, username, password_hash, full_name, phone, role_id, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID.String(), "user@example.com", "testuser", "hash", "Test User", "1234567890", 3, true, time.Now(), time.Now(),
	).Error)

	u := &model.User{
		ID:           userID,
		Email:        "user@example.com",
		Username:     "testuser",
		PasswordHash: "hash",
		FullName:     "Test User",
		Phone:        "1234567890",
		RoleID:       3,
		IsActive:     true,
	}

	userSvc := service.NewUserService(repository.NewUserRepo(db))
	userHandler := handler.NewUserHandler(userSvc)
	return userHandler, u, db
}

func TestUserHandler_UpdateMe_IgnoresExtraFieldsInJSON(t *testing.T) {
	h, u, db := setupUserHandler(t)

	// Create request JSON with extra fields that should be ignored
	// This tests that only full_name and phone are processed, even if
	// email, username, role, or other fields are present in the JSON
	reqBody := map[string]interface{}{
		"full_name": "New Name",
		"email":     "attacker@evil.com",     // should be ignored
		"username":  "attacker",              // should be ignored
		"role":      "admin",                 // should be ignored
		"is_active": false,                   // should be ignored
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/me", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", u.ID.String())

	h.UpdateMe(c)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify response indicates success
	var respBody map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &respBody)
	require.True(t, respBody["success"].(bool))
	data := respBody["data"].(map[string]interface{})
	require.Equal(t, "New Name", data["full_name"])

	// Verify the user record was updated with only full_name changed
	updated, err := repository.NewUserRepo(db).FindByID(u.ID)
	require.NoError(t, err)
	require.Equal(t, "New Name", updated.FullName)
	require.Equal(t, "user@example.com", updated.Email)      // email NOT changed
	require.Equal(t, "testuser", updated.Username)          // username NOT changed
	require.Equal(t, int16(3), updated.RoleID)              // role NOT changed
	require.True(t, updated.IsActive)                        // is_active NOT changed
	require.Equal(t, "1234567890", updated.Phone)           // phone NOT changed
}

func TestUserHandler_UpdateMe_WithPhoneAndExtraFields(t *testing.T) {
	h, u, db := setupUserHandler(t)

	// Create request JSON with phone and extra fields
	reqBody := map[string]interface{}{
		"full_name": "Updated Name",
		"phone":     "+66812345678",
		"email":     "attacker@evil.com", // should be ignored
		"role":      "admin",             // should be ignored
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/me", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", u.ID.String())

	h.UpdateMe(c)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify response contains updated fields
	var respBody map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &respBody)
	require.True(t, respBody["success"].(bool))
	data := respBody["data"].(map[string]interface{})
	require.Equal(t, "Updated Name", data["full_name"])
	require.Equal(t, "+66812345678", data["phone"])

	// Verify the user record was updated correctly
	updated, err := repository.NewUserRepo(db).FindByID(u.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated Name", updated.FullName)
	require.Equal(t, "+66812345678", updated.Phone)
	require.Equal(t, "user@example.com", updated.Email)  // email NOT changed
	require.Equal(t, "testuser", updated.Username)      // username NOT changed
	require.Equal(t, int16(3), updated.RoleID)          // role NOT changed
}
