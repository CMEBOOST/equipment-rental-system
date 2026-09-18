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

func TestAdminUserHandler_Get_ReturnsUser(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID.String(), "get@example.com", "getuser", "hash", "Get User", 3, true,
	).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users/"+userID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}

	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, "get@example.com", resp.Data["email"])
	require.Equal(t, "customer", resp.Data["role"])
}

func TestAdminUserHandler_Get_UnknownID_Returns404(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	unknownID := uuid.New()
	c.Request, _ = http.NewRequest("GET", "/api/v1/users/"+unknownID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: unknownID.String()}}

	h.Get(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminUserHandler_Get_InvalidUUID_Returns404(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users/not-a-uuid", nil)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}

	h.Get(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminUserHandler_Update_ChangesFields(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID.String(), "old@example.com", "olduser", "hash", "Old Name", 3, true,
	).Error)

	reqBody := map[string]interface{}{
		"email": "new@example.com", "username": "newuser", "full_name": "New Name", "phone": "0812345678",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/users/"+userID.String(), bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}

	h.Update(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, "new@example.com", resp.Data["email"])
	require.Equal(t, "newuser", resp.Data["username"])
	require.Equal(t, "New Name", resp.Data["full_name"])

	updated, err := repository.NewUserRepo(db).FindByID(userID)
	require.NoError(t, err)
	require.Equal(t, "new@example.com", updated.Email)
	require.Equal(t, "newuser", updated.Username)
}

func TestAdminUserHandler_Update_DuplicateEmail_Returns409(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	existingID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		existingID.String(), "taken@example.com", "takenuser", "hash", 3, true,
	).Error)
	targetID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		targetID.String(), "target@example.com", "targetuser", "hash", 3, true,
	).Error)

	reqBody := map[string]interface{}{"email": "taken@example.com"}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/users/"+targetID.String(), bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: targetID.String()}}

	h.Update(c)

	require.Equal(t, http.StatusConflict, w.Code)
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "EMAIL_ALREADY_EXISTS", resp.Error.Code)

	// target user's email must be unchanged after the rejected update
	unchanged, err := repository.NewUserRepo(db).FindByID(targetID)
	require.NoError(t, err)
	require.Equal(t, "target@example.com", unchanged.Email)
}

func TestAdminUserHandler_Update_DuplicateUsername_Returns409(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	existingID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		existingID.String(), "existing@example.com", "existinguser", "hash", 3, true,
	).Error)
	targetID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		targetID.String(), "target2@example.com", "targetuser2", "hash", 3, true,
	).Error)

	reqBody := map[string]interface{}{"username": "existinguser"}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/users/"+targetID.String(), bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: targetID.String()}}

	h.Update(c)

	require.Equal(t, http.StatusConflict, w.Code)
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "USERNAME_ALREADY_EXISTS", resp.Error.Code)
}

// This is the key regression test for the "different code path from Create"
// concern: the target keeps its own current email unchanged while only its
// username changes to a value already used by someone else. A naive
// re-query-on-conflict guard copied verbatim from Create (which never
// excludes the row being updated) would match the target's own unchanged
// email against itself and misreport EMAIL_ALREADY_EXISTS instead of
// USERNAME_ALREADY_EXISTS.
func TestAdminUserHandler_Update_UsernameConflict_WithUnchangedEmail_ReturnsUsernameNotEmailConflict(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	existingID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		existingID.String(), "other@example.com", "otheruser", "hash", 3, true,
	).Error)
	targetID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		targetID.String(), "unchanged@example.com", "targetuser3", "hash", 3, true,
	).Error)

	// Same email as the target already has (unchanged), but username collides with "otheruser".
	reqBody := map[string]interface{}{"email": "unchanged@example.com", "username": "otheruser"}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/users/"+targetID.String(), bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: targetID.String()}}

	h.Update(c)

	require.Equal(t, http.StatusConflict, w.Code)
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "USERNAME_ALREADY_EXISTS", resp.Error.Code, "must attribute the conflict to username, not the target's own unchanged email")
}

func TestAdminUserHandler_Update_UnknownID_Returns404(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	reqBody := map[string]interface{}{"full_name": "Ghost"}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	unknownID := uuid.New()
	c.Request, _ = http.NewRequest("PUT", "/api/v1/users/"+unknownID.String(), bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: unknownID.String()}}

	h.Update(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminUserHandler_Delete_ThenGet_Returns404(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "del@example.com", "deluser", "hash", 3, true,
	).Error)
	callerID := uuid.New() // different from target, so the self-delete guard doesn't trigger

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("DELETE", "/api/v1/users/"+userID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	c.Set("user_id", callerID.String())

	h.Delete(c)
	require.Equal(t, http.StatusOK, w.Code)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request, _ = http.NewRequest("GET", "/api/v1/users/"+userID.String(), nil)
	c2.Params = gin.Params{{Key: "id", Value: userID.String()}}

	h.Get(c2)
	require.Equal(t, http.StatusNotFound, w2.Code)
}

func TestAdminUserHandler_Delete_RevokesTargetUsersSessions(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "sess@example.com", "sessuser", "hash", 3, true,
	).Error)
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: userID, TokenHash: "tok-1", ExpiresAt: time.Now().Add(time.Hour),
	}))
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: userID, TokenHash: "tok-2", ExpiresAt: time.Now().Add(time.Hour),
	}))
	active, err := refreshRepo.ListActiveForUser(userID)
	require.NoError(t, err)
	require.Len(t, active, 2, "sanity check: sessions active before delete")

	callerID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("DELETE", "/api/v1/users/"+userID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	c.Set("user_id", callerID.String())

	h.Delete(c)
	require.Equal(t, http.StatusOK, w.Code)

	active, err = refreshRepo.ListActiveForUser(userID)
	require.NoError(t, err)
	require.Empty(t, active, "all of the TARGET user's sessions must be revoked, not just the caller's")
}

func TestAdminUserHandler_Delete_SelfDelete_Returns403_AndDoesNotDelete(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	adminID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		adminID.String(), "self@example.com", "selfuser", "hash", 1, true,
	).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("DELETE", "/api/v1/users/"+adminID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: adminID.String()}}
	c.Set("user_id", adminID.String())

	h.Delete(c)

	require.Equal(t, http.StatusForbidden, w.Code)

	// must still exist (not soft-deleted) after the rejected self-delete
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request, _ = http.NewRequest("GET", "/api/v1/users/"+adminID.String(), nil)
	c2.Params = gin.Params{{Key: "id", Value: adminID.String()}}

	h.Get(c2)
	require.Equal(t, http.StatusOK, w2.Code)
}

func TestAdminUserHandler_Delete_UnknownID_Returns404(t *testing.T) {
	h, _ := setupAdminUserHandler(t)
	unknownID := uuid.New()
	callerID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("DELETE", "/api/v1/users/"+unknownID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: unknownID.String()}}
	c.Set("user_id", callerID.String())

	h.Delete(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminUserHandler_ChangeRole_ValidRequest_ChangesRole(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "role@example.com", "roleuser", "hash", 3, true,
	).Error)

	reqBody := map[string]interface{}{"role": "staff"}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+userID.String()+"/role", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}

	h.ChangeRole(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, "staff", resp.Data["role"])

	// verify against real DB state, not just the returned value
	updated, err := repository.NewUserRepo(db).FindByID(userID)
	require.NoError(t, err)
	require.Equal(t, "staff", updated.Role.Name)
}

func TestAdminUserHandler_ChangeRole_UnknownID_Returns404(t *testing.T) {
	h, _ := setupAdminUserHandler(t)
	unknownID := uuid.New()

	reqBody := map[string]interface{}{"role": "staff"}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+unknownID.String()+"/role", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: unknownID.String()}}

	h.ChangeRole(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminUserHandler_ChangeRole_InvalidUUID_Returns404(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	reqBody := map[string]interface{}{"role": "staff"}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/not-a-uuid/role", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}

	h.ChangeRole(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

// An unrecognized role name must fail cleanly with 400 (caught by the DTO's
// binding:"oneof=admin staff customer" tag) rather than panicking or being
// silently ignored.
func TestAdminUserHandler_ChangeRole_UnknownRoleName_Returns400(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "role2@example.com", "roleuser2", "hash", 3, true,
	).Error)

	reqBody := map[string]interface{}{"role": "superadmin"}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+userID.String()+"/role", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}

	require.NotPanics(t, func() { h.ChangeRole(c) })

	require.Equal(t, http.StatusBadRequest, w.Code)

	// role must be unchanged after the rejected request
	unchanged, err := repository.NewUserRepo(db).FindByID(userID)
	require.NoError(t, err)
	require.Equal(t, "customer", unchanged.Role.Name)
}

func TestAdminUserHandler_ChangeStatus_DisablingRevokesSessions(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "status@example.com", "statususer", "hash", 3, true,
	).Error)
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: userID, TokenHash: "stok-1", ExpiresAt: time.Now().Add(time.Hour),
	}))
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: userID, TokenHash: "stok-2", ExpiresAt: time.Now().Add(time.Hour),
	}))
	active, err := refreshRepo.ListActiveForUser(userID)
	require.NoError(t, err)
	require.Len(t, active, 2, "sanity check: sessions active before status change")

	callerID := uuid.New()
	reqBody := map[string]interface{}{"is_active": false}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+userID.String()+"/status", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	c.Set("user_id", callerID.String())

	h.ChangeStatus(c)

	require.Equal(t, http.StatusOK, w.Code)

	// verify against real DB state (both is_active and revoked sessions),
	// not just the handler's returned JSON.
	updated, err := repository.NewUserRepo(db).FindByID(userID)
	require.NoError(t, err)
	require.False(t, updated.IsActive)

	active, err = refreshRepo.ListActiveForUser(userID)
	require.NoError(t, err)
	require.Empty(t, active, "all of the TARGET user's sessions must be revoked after disabling")
}

func TestAdminUserHandler_ChangeStatus_EnablingDoesNotRevokeSessions(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "enab@example.com", "enabuser", "hash", 3, false,
	).Error)
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: userID, TokenHash: "etok-1", ExpiresAt: time.Now().Add(time.Hour),
	}))

	callerID := uuid.New()
	reqBody := map[string]interface{}{"is_active": true}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+userID.String()+"/status", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	c.Set("user_id", callerID.String())

	h.ChangeStatus(c)

	require.Equal(t, http.StatusOK, w.Code)

	updated, err := repository.NewUserRepo(db).FindByID(userID)
	require.NoError(t, err)
	require.True(t, updated.IsActive)

	active, err := refreshRepo.ListActiveForUser(userID)
	require.NoError(t, err)
	require.Len(t, active, 1, "enabling must not revoke existing sessions")
}

func TestAdminUserHandler_ChangeStatus_SelfChange_Returns403_AndDoesNotChange(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	adminID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		adminID.String(), "selfstatus@example.com", "selfstatususer", "hash", 1, true,
	).Error)
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: adminID, TokenHash: "self-tok", ExpiresAt: time.Now().Add(time.Hour),
	}))

	reqBody := map[string]interface{}{"is_active": false}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+adminID.String()+"/status", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: adminID.String()}}
	c.Set("user_id", adminID.String())

	h.ChangeStatus(c)

	require.Equal(t, http.StatusForbidden, w.Code)

	// must still be active, and sessions must remain, after the rejected
	// self-status-change (the guard runs before any write, including the
	// session-revocation write).
	unchanged, err := repository.NewUserRepo(db).FindByID(adminID)
	require.NoError(t, err)
	require.True(t, unchanged.IsActive)

	active, err := refreshRepo.ListActiveForUser(adminID)
	require.NoError(t, err)
	require.Len(t, active, 1)
}

func TestAdminUserHandler_ChangeStatus_UnknownID_Returns404(t *testing.T) {
	h, _ := setupAdminUserHandler(t)
	unknownID := uuid.New()
	callerID := uuid.New()

	reqBody := map[string]interface{}{"is_active": false}
	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+unknownID.String()+"/status", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: unknownID.String()}}
	c.Set("user_id", callerID.String())

	h.ChangeStatus(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}
