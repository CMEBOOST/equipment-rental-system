package router_test

import (
	"bytes"
	"encoding/json"
	"io"
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
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/router"
	"github.com/equipment-rental-system/user-service/internal/service"
)

// RequireRole was only ever tested in isolation, which proves the middleware
// works but says nothing about whether each route is wired to the right
// roles -- the mistake that actually ships. These tests drive real requests
// through the assembled engine for a representative sample of routes.

const routerInternalKey = "router-internal-key"

func newTestRouter(t *testing.T) (*gin.Engine, *config.Config, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard

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
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_users_email_active ON users(email) WHERE deleted_at IS NULL`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_users_username_active ON users(username) WHERE deleted_at IS NULL`).Error)
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

	cfg := &config.Config{
		BCryptCost: 4, JWTSecret: "test-secret-min-32-characters-ok",
		JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 168 * time.Hour,
		InternalAPIKey: routerInternalKey, CORSOrigin: "http://localhost:3000",
	}
	return router.New(db, cfg), cfg, db
}

// tokenFor mints a valid access token carrying the given role, exactly as a
// successful login would.
func tokenFor(t *testing.T, cfg *config.Config, role string) string {
	t.Helper()
	ts := service.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL)
	token, _, err := ts.GenerateAccessToken(model.User{
		ID: uuid.New(), Email: role + "@example.com", Username: role + "user",
		Role: model.Role{Name: role},
	})
	require.NoError(t, err)
	return token
}

func seedUser(t *testing.T, db *gorm.DB, email, username string, roleID int) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, phone, role_id, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), email, username, "hash", "Seeded User", "", roleID, true, time.Now(), time.Now(),
	).Error)
	return id
}

func do(r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	var reader io.Reader = bytes.NewReader([]byte(`{}`))
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func requireCode(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, want, resp.Error.Code)
}

// ---------- POST /users : admin only ----------

func TestRouter_CreateUser_AdminAllowed(t *testing.T) {
	r, cfg, _ := newTestRouter(t)

	w := do(r, "POST", "/api/v1/users", tokenFor(t, cfg, "admin"), map[string]any{
		"email": "made@example.com", "username": "madeuser", "password": "Passw0rd1", "role": "staff",
	})

	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
}

func TestRouter_CreateUser_CustomerForbidden(t *testing.T) {
	r, cfg, _ := newTestRouter(t)

	w := do(r, "POST", "/api/v1/users", tokenFor(t, cfg, "customer"), map[string]any{
		"email": "nope@example.com", "username": "nopeuser", "password": "Passw0rd1", "role": "staff",
	})

	require.Equal(t, http.StatusForbidden, w.Code)
	requireCode(t, w, "FORBIDDEN")
}

func TestRouter_CreateUser_StaffForbidden(t *testing.T) {
	r, cfg, _ := newTestRouter(t)

	w := do(r, "POST", "/api/v1/users", tokenFor(t, cfg, "staff"), map[string]any{
		"email": "nope2@example.com", "username": "nope2user", "password": "Passw0rd1", "role": "staff",
	})

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestRouter_CreateUser_NoToken_Returns401(t *testing.T) {
	r, _, _ := newTestRouter(t)

	w := do(r, "POST", "/api/v1/users", "", map[string]any{
		"email": "nope3@example.com", "username": "nope3user", "password": "Passw0rd1", "role": "staff",
	})

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireCode(t, w, "UNAUTHENTICATED")
}

// ---------- GET /users/{id} : admin + staff ----------

func TestRouter_GetUser_StaffAllowed(t *testing.T) {
	r, cfg, db := newTestRouter(t)
	id := seedUser(t, db, "target@example.com", "targetuser", 3)

	w := do(r, "GET", "/api/v1/users/"+id.String(), tokenFor(t, cfg, "staff"), nil)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

func TestRouter_GetUser_CustomerForbidden(t *testing.T) {
	r, cfg, db := newTestRouter(t)
	id := seedUser(t, db, "target2@example.com", "target2user", 3)

	w := do(r, "GET", "/api/v1/users/"+id.String(), tokenFor(t, cfg, "customer"), nil)

	require.Equal(t, http.StatusForbidden, w.Code)
	requireCode(t, w, "FORBIDDEN")
}

// ---------- PUT /users/{id} : admin only (staff must NOT be able to edit) ----------

func TestRouter_UpdateUser_StaffForbidden(t *testing.T) {
	r, cfg, db := newTestRouter(t)
	id := seedUser(t, db, "target3@example.com", "target3user", 3)

	w := do(r, "PUT", "/api/v1/users/"+id.String(), tokenFor(t, cfg, "staff"),
		map[string]any{"full_name": "Renamed By Staff"})

	require.Equal(t, http.StatusForbidden, w.Code)
	requireCode(t, w, "FORBIDDEN")

	// And the write really did not happen.
	var name string
	require.NoError(t, db.Raw(`SELECT full_name FROM users WHERE id = ?`, id.String()).Scan(&name).Error)
	require.NotEqual(t, "Renamed By Staff", name)
}

func TestRouter_UpdateUser_AdminAllowed(t *testing.T) {
	r, cfg, db := newTestRouter(t)
	id := seedUser(t, db, "target4@example.com", "target4user", 3)

	w := do(r, "PUT", "/api/v1/users/"+id.String(), tokenFor(t, cfg, "admin"),
		map[string]any{"full_name": "Renamed By Admin"})

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

// ---------- DELETE /users/{id} and PATCH /users/{id}/role : admin only ----------

func TestRouter_DeleteUser_StaffForbidden(t *testing.T) {
	r, cfg, db := newTestRouter(t)
	id := seedUser(t, db, "target5@example.com", "target5user", 3)

	w := do(r, "DELETE", "/api/v1/users/"+id.String(), tokenFor(t, cfg, "staff"), nil)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestRouter_ChangeRole_StaffForbidden(t *testing.T) {
	r, cfg, db := newTestRouter(t)
	id := seedUser(t, db, "target6@example.com", "target6user", 3)

	w := do(r, "PATCH", "/api/v1/users/"+id.String()+"/role", tokenFor(t, cfg, "staff"),
		map[string]any{"role": "admin"})

	require.Equal(t, http.StatusForbidden, w.Code)
}

// ---------- GET /roles : admin + staff ----------

func TestRouter_ListRoles_StaffAllowed_CustomerForbidden(t *testing.T) {
	r, cfg, _ := newTestRouter(t)

	staffResp := do(r, "GET", "/api/v1/roles", tokenFor(t, cfg, "staff"), nil)
	require.Equal(t, http.StatusOK, staffResp.Code)

	customerResp := do(r, "GET", "/api/v1/roles", tokenFor(t, cfg, "customer"), nil)
	require.Equal(t, http.StatusForbidden, customerResp.Code)
}

// ---------- /me : any authenticated role, no RequireRole ----------

func TestRouter_Me_CustomerAllowed(t *testing.T) {
	r, cfg, db := newTestRouter(t)
	id := seedUser(t, db, "self@example.com", "selfuser", 3)
	ts := service.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL)
	token, _, err := ts.GenerateAccessToken(model.User{
		ID: id, Email: "self@example.com", Username: "selfuser", Role: model.Role{Name: "customer"},
	})
	require.NoError(t, err)

	w := do(r, "GET", "/api/v1/me", token, nil)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

func TestRouter_Me_NoToken_Returns401(t *testing.T) {
	r, _, _ := newTestRouter(t)

	w := do(r, "GET", "/api/v1/me", "", nil)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------- public and internal routes ----------

func TestRouter_Register_IsPublic(t *testing.T) {
	r, _, _ := newTestRouter(t)

	w := do(r, "POST", "/api/v1/auth/register", "", map[string]any{
		"email": "public@example.com", "username": "publicuser", "password": "Passw0rd1",
	})

	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
}

func TestRouter_Verify_RequiresInternalKeyNotBearerToken(t *testing.T) {
	r, cfg, _ := newTestRouter(t)

	// An ordinary admin bearer token is not what guards this route.
	w := do(r, "POST", "/api/v1/auth/verify", tokenFor(t, cfg, "admin"), map[string]any{"token": "whatever"})
	require.Equal(t, http.StatusForbidden, w.Code)
	requireCode(t, w, "FORBIDDEN")

	// With the internal key it gets as far as token validation.
	req, _ := http.NewRequest("POST", "/api/v1/auth/verify", bytes.NewReader([]byte(`{"token":"garbage"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", routerInternalKey)
	keyed := httptest.NewRecorder()
	r.ServeHTTP(keyed, req)
	require.Equal(t, http.StatusUnauthorized, keyed.Code)
	requireCode(t, keyed, "UNAUTHENTICATED")
}

func TestRouter_Health_IsPublic(t *testing.T) {
	r, _, _ := newTestRouter(t)

	w := do(r, "GET", "/health", "", nil)

	require.Equal(t, http.StatusOK, w.Code)
}

// The CORS middleware is only useful if it is actually installed on the
// assembled engine, including for preflights on routes that register no
// OPTIONS handler.
func TestRouter_CORSHeadersArePresent(t *testing.T) {
	r, cfg, _ := newTestRouter(t)

	w := do(r, "GET", "/health", "", nil)
	require.Equal(t, cfg.CORSOrigin, w.Header().Get("Access-Control-Allow-Origin"))

	req, _ := http.NewRequest("OPTIONS", "/api/v1/users", nil)
	req.Header.Set("Origin", cfg.CORSOrigin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	pre := httptest.NewRecorder()
	r.ServeHTTP(pre, req)

	require.Equal(t, http.StatusNoContent, pre.Code)
	require.Equal(t, cfg.CORSOrigin, pre.Header().Get("Access-Control-Allow-Origin"))
}
