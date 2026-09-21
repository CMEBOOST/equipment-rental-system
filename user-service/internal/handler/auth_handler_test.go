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
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/handler"
	"github.com/equipment-rental-system/user-service/internal/middleware"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

// The auth endpoints were only ever tested at the service layer, so the
// HTTP status / error.code mapping -- the part other services actually
// integrate against -- was never exercised. These tests drive each of the
// five endpoints through its handler (and, for /auth/verify, through the
// internal-key middleware that guards it).

const testInternalKey = "test-internal-key"

func setupAuthHandler(t *testing.T) (*handler.AuthHandler, *config.Config, *gorm.DB) {
	t.Helper()
	// TranslateError makes unique-constraint violations surface as
	// gorm.ErrDuplicatedKey, which Register's TOCTOU guard relies on.
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
			email TEXT,
			username TEXT,
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
	applyUserUniqueIndexes(t, db)

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
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)

	cfg := &config.Config{
		BCryptCost: 4, JWTSecret: "test-secret-min-32-characters-ok",
		JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 168 * time.Hour,
		InternalAPIKey: testInternalKey,
	}
	authSvc := service.NewAuthService(
		repository.NewUserRepo(db), repository.NewRoleRepo(db),
		repository.NewRefreshTokenRepo(db), repository.NewLoginLogRepo(db),
		service.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL), cfg,
	)
	return handler.NewAuthHandler(authSvc), cfg, db
}

// doJSON invokes a handler with a JSON body, the way an incoming request
// would reach it.
func doJSON(h gin.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader([]byte(`{}`))
	} else {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	c.Request, _ = http.NewRequest(method, path, reader)
	c.Request.Header.Set("Content-Type", "application/json")
	h(c)
	return w
}

func decodeData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success, "body: %s", w.Body.String())
	return resp.Data
}

// registerAndLogin returns a fresh user's access and refresh tokens.
func registerAndLogin(t *testing.T, h *handler.AuthHandler, email, username string) (access, refresh string) {
	t.Helper()
	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": email, "username": username, "password": "Passw0rd1", "full_name": "T U",
	})
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

	w = doJSON(h.Login, "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Passw0rd1",
	})
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	data := decodeData(t, w)
	return data["access_token"].(string), data["refresh_token"].(string)
}

// ---------- POST /auth/register ----------

func TestAuthHandler_Register_Valid_Returns201WithCustomerRole(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "reg@example.com", "username": "reguser", "password": "Passw0rd1",
		"full_name": "Reg User", "phone": "0812345678",
	})

	require.Equal(t, http.StatusCreated, w.Code)
	data := decodeData(t, w)
	require.Equal(t, "reg@example.com", data["email"])
	require.Equal(t, "customer", data["role"], "self-registration must always assign the customer role")
	require.NotEmpty(t, data["id"])
}

func TestAuthHandler_Register_WeakPassword_Returns422(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	// Long enough for the DTO's min=8, but no digit -- only the service's
	// strength check can reject it.
	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "weak@example.com", "username": "weakuser", "password": "allletters",
	})

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "WEAK_PASSWORD")
}

func TestAuthHandler_Register_DuplicateEmail_Returns409(t *testing.T) {
	h, _, _ := setupAuthHandler(t)
	registerAndLogin(t, h, "dup@example.com", "dupuser")

	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "dup@example.com", "username": "otheruser", "password": "Passw0rd1",
	})

	require.Equal(t, http.StatusConflict, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "EMAIL_ALREADY_EXISTS")
}

func TestAuthHandler_Register_DuplicateUsername_Returns409(t *testing.T) {
	h, _, _ := setupAuthHandler(t)
	registerAndLogin(t, h, "first@example.com", "takenname")

	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "second@example.com", "username": "takenname", "password": "Passw0rd1",
	})

	require.Equal(t, http.StatusConflict, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "USERNAME_ALREADY_EXISTS")
}

func TestAuthHandler_Register_MalformedEmail_Returns400(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "not-an-email", "username": "baduser", "password": "Passw0rd1",
	})

	require.Equal(t, http.StatusBadRequest, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

// ---------- POST /auth/login ----------

func TestAuthHandler_Login_Valid_Returns200WithTokenPair(t *testing.T) {
	h, _, _ := setupAuthHandler(t)
	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "login@example.com", "username": "loginuser", "password": "Passw0rd1",
	})
	require.Equal(t, http.StatusCreated, w.Code)

	w = doJSON(h.Login, "POST", "/api/v1/auth/login", map[string]any{
		"email": "login@example.com", "password": "Passw0rd1",
	})

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	require.NotEmpty(t, data["access_token"])
	require.NotEmpty(t, data["refresh_token"])
	require.Equal(t, "Bearer", data["token_type"])
	require.NotZero(t, data["expires_in"])
	user := data["user"].(map[string]any)
	require.Equal(t, "login@example.com", user["email"])
	require.Equal(t, "customer", user["role"])
}

func TestAuthHandler_Login_WrongPassword_Returns401(t *testing.T) {
	h, _, _ := setupAuthHandler(t)
	registerAndLogin(t, h, "wrongpw@example.com", "wrongpwuser")

	w := doJSON(h.Login, "POST", "/api/v1/auth/login", map[string]any{
		"email": "wrongpw@example.com", "password": "NotTheP4ssword",
	})

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INVALID_CREDENTIALS")
}

func TestAuthHandler_Login_UnknownEmail_Returns401(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Login, "POST", "/api/v1/auth/login", map[string]any{
		"email": "nobody@example.com", "password": "Passw0rd1",
	})

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INVALID_CREDENTIALS")
}

func TestAuthHandler_Login_DisabledAccount_Returns403(t *testing.T) {
	h, _, db := setupAuthHandler(t)
	registerAndLogin(t, h, "disabled@example.com", "disableduser")
	require.NoError(t, db.Exec(`UPDATE users SET is_active = ? WHERE email = ?`, false, "disabled@example.com").Error)

	w := doJSON(h.Login, "POST", "/api/v1/auth/login", map[string]any{
		"email": "disabled@example.com", "password": "Passw0rd1",
	})

	require.Equal(t, http.StatusForbidden, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "ACCOUNT_DISABLED")
}

// ---------- POST /auth/refresh ----------

func TestAuthHandler_Refresh_ValidToken_Returns200WithRotatedPair(t *testing.T) {
	h, _, _ := setupAuthHandler(t)
	_, refresh := registerAndLogin(t, h, "refresh@example.com", "refreshuser")

	w := doJSON(h.Refresh, "POST", "/api/v1/auth/refresh", map[string]any{"refresh_token": refresh})

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	require.NotEmpty(t, data["access_token"])
	require.NotEqual(t, refresh, data["refresh_token"], "the presented refresh token must be rotated, not reissued")
}

func TestAuthHandler_Refresh_InvalidToken_Returns401(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Refresh, "POST", "/api/v1/auth/refresh", map[string]any{"refresh_token": "not-a-real-token"})

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INVALID_REFRESH_TOKEN")
}

func TestAuthHandler_Refresh_ReusedToken_Returns401(t *testing.T) {
	h, _, _ := setupAuthHandler(t)
	_, refresh := registerAndLogin(t, h, "reuse-rt@example.com", "reusertuser")
	first := doJSON(h.Refresh, "POST", "/api/v1/auth/refresh", map[string]any{"refresh_token": refresh})
	require.Equal(t, http.StatusOK, first.Code)

	// The same token a second time: it was revoked by the rotation above.
	w := doJSON(h.Refresh, "POST", "/api/v1/auth/refresh", map[string]any{"refresh_token": refresh})

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INVALID_REFRESH_TOKEN")
}

func TestAuthHandler_Refresh_MissingToken_Returns400(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Refresh, "POST", "/api/v1/auth/refresh", map[string]any{})

	require.Equal(t, http.StatusBadRequest, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

// ---------- POST /auth/logout ----------

func TestAuthHandler_Logout_Valid_Returns200AndRefreshStopsWorking(t *testing.T) {
	h, _, _ := setupAuthHandler(t)
	_, refresh := registerAndLogin(t, h, "logout@example.com", "logoutuser")

	w := doJSON(h.Logout, "POST", "/api/v1/auth/logout", map[string]any{"refresh_token": refresh})

	require.Equal(t, http.StatusOK, w.Code)
	require.NotEmpty(t, decodeData(t, w)["message"])

	after := doJSON(h.Refresh, "POST", "/api/v1/auth/refresh", map[string]any{"refresh_token": refresh})
	require.Equal(t, http.StatusUnauthorized, after.Code, "the revoked token must no longer refresh")
}

func TestAuthHandler_Logout_MissingToken_Returns400(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Logout, "POST", "/api/v1/auth/logout", map[string]any{})

	require.Equal(t, http.StatusBadRequest, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

// ---------- POST /auth/verify (internal) ----------

// verifyEngine wires Verify behind the same RequireInternalKey middleware
// the router uses, so the internal-key rejection is exercised as a real
// request rather than as a unit test of the middleware in isolation.
func verifyEngine(h *handler.AuthHandler, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/auth/verify", middleware.RequireInternalKey(cfg.InternalAPIKey), h.Verify)
	return r
}

func postVerify(r *gin.Engine, key string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/v1/auth/verify", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthHandler_Verify_ValidTokenAndKey_Returns200WithIdentity(t *testing.T) {
	h, cfg, _ := setupAuthHandler(t)
	access, _ := registerAndLogin(t, h, "verify@example.com", "verifyuser")
	r := verifyEngine(h, cfg)

	w := postVerify(r, testInternalKey, map[string]any{"token": access})

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	require.Equal(t, true, data["active"])
	require.Equal(t, "verify@example.com", data["email"])
	require.Equal(t, "customer", data["role"])
	require.NotEmpty(t, data["user_id"])
	require.NotEmpty(t, data["expires_at"])
}

func TestAuthHandler_Verify_WrongInternalKey_Returns403(t *testing.T) {
	h, cfg, _ := setupAuthHandler(t)
	access, _ := registerAndLogin(t, h, "verify403@example.com", "verify403")
	r := verifyEngine(h, cfg)

	w := postVerify(r, "wrong-key", map[string]any{"token": access})

	require.Equal(t, http.StatusForbidden, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestAuthHandler_Verify_MissingInternalKey_Returns403(t *testing.T) {
	h, cfg, _ := setupAuthHandler(t)
	access, _ := registerAndLogin(t, h, "verifynokey@example.com", "verifynokey")
	r := verifyEngine(h, cfg)

	w := postVerify(r, "", map[string]any{"token": access})

	require.Equal(t, http.StatusForbidden, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestAuthHandler_Verify_InvalidToken_Returns401(t *testing.T) {
	h, cfg, _ := setupAuthHandler(t)
	r := verifyEngine(h, cfg)

	w := postVerify(r, testInternalKey, map[string]any{"token": "garbage.token.value"})

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "UNAUTHENTICATED")
}

func TestAuthHandler_Verify_DisabledAccount_Returns403AccountDisabled(t *testing.T) {
	h, cfg, db := setupAuthHandler(t)
	access, _ := registerAndLogin(t, h, "verifydisabled@example.com", "verifydisabled")
	require.NoError(t, db.Exec(`UPDATE users SET is_active = ? WHERE email = ?`, false, "verifydisabled@example.com").Error)
	r := verifyEngine(h, cfg)

	w := postVerify(r, testInternalKey, map[string]any{"token": access})

	require.Equal(t, http.StatusForbidden, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "ACCOUNT_DISABLED")
}
