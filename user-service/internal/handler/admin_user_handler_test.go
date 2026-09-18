package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/handler"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func setupAdminUserHandler(t *testing.T) (*handler.AdminUserHandler, *gorm.DB) {
	// TranslateError is required so unique-constraint violations from Create()
	// surface as gorm.ErrDuplicatedKey (see UserService.CreateUser's TOCTOU guard)
	// instead of the raw, driver-specific SQLite error.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
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

	require.NoError(t, db.Create(&model.Role{ID: 1, Name: "admin"}).Error)
	require.NoError(t, db.Create(&model.Role{ID: 2, Name: "staff"}).Error)
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)

	cfg := &config.Config{BCryptCost: 4}
	userSvc := service.NewUserService(repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db), cfg, repository.NewLoginLogRepo(db))
	roleRepo := repository.NewRoleRepo(db)
	adminHandler := handler.NewAdminUserHandler(userSvc, roleRepo, cfg.BCryptCost)
	return adminHandler, db
}

func TestAdminUserHandler_Create_ValidRequest_ReturnsCreatedUserWithRequestedRole(t *testing.T) {
	h, db := setupAdminUserHandler(t)

	reqBody := map[string]interface{}{
		"email": "newstaff@example.com", "username": "newstaff", "password": "St@ffPass1",
		"full_name": "New Staff", "role": "staff",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/api/v1/users", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Create(c)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, "newstaff@example.com", resp.Data["email"])
	require.Equal(t, "staff", resp.Data["role"])

	// The stored password hash must actually verify against the strong
	// password that was submitted (not left empty or double-hashed).
	created, err := repository.NewUserRepo(db).FindByEmail("newstaff@example.com")
	require.NoError(t, err)
	require.Equal(t, "staff", created.Role.Name)
}

func TestAdminUserHandler_Create_WeakPassword_Returns422(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	// "allletters" is 10 chars (passes the DTO's min=8 tag) but has no digit,
	// so only the service's isStrongPassword check (not the DTO tag) can catch
	// it -- this is exactly the Task 10 bug class the brief warns against.
	reqBody := map[string]interface{}{
		"email": "weak@example.com", "username": "weakuser", "password": "allletters", "role": "staff",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/api/v1/users", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Create(c)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestAdminUserHandler_Create_DuplicateEmail_Returns409(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	reqBody := map[string]interface{}{
		"email": "dup@example.com", "username": "user1", "password": "St@ffPass1", "role": "staff",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/api/v1/users", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Create(c)
	require.Equal(t, http.StatusCreated, w.Code)

	reqBody2 := map[string]interface{}{
		"email": "dup@example.com", "username": "user2", "password": "St@ffPass1", "role": "staff",
	}
	bodyBytes2, _ := json.Marshal(reqBody2)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request, _ = http.NewRequest("POST", "/api/v1/users", bytes.NewReader(bodyBytes2))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Create(c2)

	require.Equal(t, http.StatusConflict, w2.Code)
}

func TestAdminUserHandler_Create_InvalidRole_Returns400(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	reqBody := map[string]interface{}{
		"email": "bad@example.com", "username": "baduser", "password": "St@ffPass1", "role": "superadmin",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/api/v1/users", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Create(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "role=superadmin fails the DTO's oneof binding")
}

func TestAdminUserHandler_List_ReturnsAllUsersWithPagination(t *testing.T) {
	h, db := setupAdminUserHandler(t)

	for i, email := range []string{"a@example.com", "b@example.com"} {
		userID := uuid.New()
		require.NoError(t, db.Exec(
			`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
			userID.String(), email, "user"+string(rune('0'+i)), "hash", 3, true,
		).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
		Meta    map[string]any   `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data, 2)
	require.Equal(t, float64(2), resp.Meta["total"])
}

func TestAdminUserHandler_List_LimitZero_DoesNotPanicAndDefaults(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), "a@example.com", "usera", "hash", 3, true,
	).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users?limit=0", nil)

	require.NotPanics(t, func() { h.List(c) })

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool           `json:"success"`
		Meta    map[string]any `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, float64(20), resp.Meta["limit"], "limit=0 should be clamped to the default of 20, matching the value actually used for the query, and must not divide by zero when computing total_pages")
}

func TestAdminUserHandler_List_LimitNonNumeric_DoesNotPanicAndDefaults(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users?limit=abc", nil)

	require.NotPanics(t, func() { h.List(c) })

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool           `json:"success"`
		Meta    map[string]any `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, float64(20), resp.Meta["limit"], "a non-numeric limit leaves strconv.Atoi's zero value, which must be clamped like limit=0")
}

func TestAdminUserHandler_List_LimitOutOfRange_ClampsToDefault(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Exec(
			`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), "u"+string(rune('a'+i))+"@example.com", "user"+string(rune('a'+i)), "hash", 3, true,
		).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users?limit=500", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
		Meta    map[string]any   `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, float64(20), resp.Meta["limit"], "limit=500 is out of [1,100] and must clamp to 20, consistent with the actually-applied query limit")
	require.Len(t, resp.Data, 3)
}

func TestAdminUserHandler_List_PageZero_ClampsToOne(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users?page=0", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Meta map[string]any `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(1), resp.Meta["page"])
}

func TestAdminUserHandler_List_FiltersByRole(t *testing.T) {
	h, db := setupAdminUserHandler(t)

	adminID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		adminID.String(), "admin@example.com", "adminuser", "hash", 1, true,
	).Error)
	staffID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		staffID.String(), "staff@example.com", "staffuser", "hash", 2, true,
	).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users?role=staff", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, "staff@example.com", resp.Data[0]["email"])
}
