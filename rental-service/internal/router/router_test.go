package router_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/rental-service/internal/config"
	"github.com/equipment-rental-system/rental-service/internal/router"
)

// ---- newTestRouter ---------------------------------------------------------

// newTestRouter wires a real router.New(database, cfg) -- SQLite in-memory
// repository plus httptest-backed product/user services that always answer
// with a generic success, so role-gating (not business logic) is what's
// under test here.
func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE rentals (
			id          TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL,
			product_id  TEXT NOT NULL,
			start_date  DATETIME NOT NULL,
			due_date    DATETIME NOT NULL,
			return_date DATETIME,
			total_price NUMERIC NOT NULL,
			status      TEXT NOT NULL,
			created_at  DATETIME,
			updated_at  DATETIME
		)
	`).Error)

	productSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"id":"00000000-0000-0000-0000-000000000001","price_per_day":100,"status":"available"}}`))
	}))
	t.Cleanup(productSrv.Close)
	userSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"active":true}}`))
	}))
	t.Cleanup(userSrv.Close)

	cfg := &config.Config{
		CORSOrigin:        "http://localhost:3000",
		InternalAPIKey:    "router-internal-key",
		ProductServiceURL: productSrv.URL,
		UserServiceURL:    userSrv.URL,
	}
	return router.New(db, cfg)
}

// ---- fakeToken --------------------------------------------------------------
// Duplicated verbatim from internal/middleware/auth_test.go (Task 8). Both
// files are in different black-box test packages, so each needs its own
// copy; they must stay byte-for-byte identical.

// fakeToken builds a base64url three-segment string shaped like a JWT.
// RequireAuth only decodes the payload segment (Kong has already verified
// the real signature/expiry before forwarding here), so the header and
// signature segments never need to be cryptographically valid -- they only
// need to exist, so the "len(parts) != 3" check in RequireAuth passes.
func fakeToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	require.NoError(t, err)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signature := base64.RawURLEncoding.EncodeToString([]byte("unsigned"))
	return header + "." + payload + "." + signature
}

// ---- do ---------------------------------------------------------------------

// do issues a real HTTP request against the assembled engine, attaching a
// Bearer header when token is non-empty and a JSON body when body is non-nil.
func do(r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---- role helpers -------------------------------------------------------

func adminToken(t *testing.T) string {
	t.Helper()
	return fakeToken(t, map[string]any{"sub": uuid.New().String(), "role": "admin"})
}
func staffToken(t *testing.T) string {
	t.Helper()
	return fakeToken(t, map[string]any{"sub": uuid.New().String(), "role": "staff"})
}
func customerToken(t *testing.T) string {
	t.Helper()
	return fakeToken(t, map[string]any{"sub": uuid.New().String(), "role": "customer"})
}

// decodeBody unmarshals a recorded JSON response body into a generic map.
func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// =============================================================================
// GET /api/v1/rentals -- admin/staff only
// =============================================================================

func TestRouter_ListRentals_AdminAllowed(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodGet, "/api/v1/rentals", adminToken(t), nil)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ListRentals_StaffAllowed(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodGet, "/api/v1/rentals", staffToken(t), nil)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ListRentals_CustomerForbidden(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodGet, "/api/v1/rentals", customerToken(t), nil)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestRouter_ListRentals_NoToken_Returns401(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodGet, "/api/v1/rentals", "", nil)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// =============================================================================
// POST /api/v1/rentals/request -- customer only
// =============================================================================

func TestRouter_RequestRental_CustomerAllowed(t *testing.T) {
	r := newTestRouter(t)
	body := map[string]any{
		"product_id": uuid.New().String(),
		"start_date": "2026-01-01",
		"due_date":   "2026-01-05",
	}

	w := do(r, http.MethodPost, "/api/v1/rentals/request", customerToken(t), body)

	require.Equal(t, http.StatusCreated, w.Code)
}

func TestRouter_RequestRental_AdminForbidden(t *testing.T) {
	r := newTestRouter(t)
	body := map[string]any{
		"product_id": uuid.New().String(),
		"start_date": "2026-01-01",
		"due_date":   "2026-01-05",
	}

	w := do(r, http.MethodPost, "/api/v1/rentals/request", adminToken(t), body)

	require.Equal(t, http.StatusForbidden, w.Code)
}

// =============================================================================
// POST /api/v1/rentals -- admin/staff only
// =============================================================================

func TestRouter_CreateRental_AdminAllowed(t *testing.T) {
	r := newTestRouter(t)
	body := map[string]any{
		"user_id":    uuid.New().String(),
		"product_id": uuid.New().String(),
		"start_date": "2026-01-01",
		"due_date":   "2026-01-05",
	}

	w := do(r, http.MethodPost, "/api/v1/rentals", adminToken(t), body)

	require.Equal(t, http.StatusCreated, w.Code)
}

func TestRouter_CreateRental_CustomerForbidden(t *testing.T) {
	r := newTestRouter(t)
	body := map[string]any{
		"user_id":    uuid.New().String(),
		"product_id": uuid.New().String(),
		"start_date": "2026-01-01",
		"due_date":   "2026-01-05",
	}

	w := do(r, http.MethodPost, "/api/v1/rentals", customerToken(t), body)

	require.Equal(t, http.StatusForbidden, w.Code)
}

// =============================================================================
// PATCH /api/v1/rentals/:id/approve -- admin/staff only
// =============================================================================

func TestRouter_ApproveRental_StaffAllowed(t *testing.T) {
	r := newTestRouter(t)
	// Seed a pending rental through the real request-a-rental flow so the
	// approval below has something valid to act on.
	requestBody := map[string]any{
		"product_id": uuid.New().String(),
		"start_date": "2026-01-01",
		"due_date":   "2026-01-05",
	}
	created := do(r, http.MethodPost, "/api/v1/rentals/request", customerToken(t), requestBody)
	require.Equal(t, http.StatusCreated, created.Code)
	data, ok := decodeBody(t, created)["data"].(map[string]any)
	require.True(t, ok)
	rentalID, ok := data["id"].(string)
	require.True(t, ok)

	w := do(r, http.MethodPatch, "/api/v1/rentals/"+rentalID+"/approve", staffToken(t), nil)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ApproveRental_CustomerForbidden(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodPatch, "/api/v1/rentals/"+uuid.New().String()+"/approve", customerToken(t), nil)

	require.Equal(t, http.StatusForbidden, w.Code)
}

// =============================================================================
// PATCH /api/v1/rentals/:id/return -- admin/staff only
// =============================================================================

func TestRouter_ReturnRental_AdminAllowed(t *testing.T) {
	r := newTestRouter(t)
	// Seed an active rental through the real create flow so the return
	// below has something valid to act on.
	createBody := map[string]any{
		"user_id":    uuid.New().String(),
		"product_id": uuid.New().String(),
		"start_date": "2026-01-01",
		"due_date":   "2026-01-05",
	}
	created := do(r, http.MethodPost, "/api/v1/rentals", adminToken(t), createBody)
	require.Equal(t, http.StatusCreated, created.Code)
	data, ok := decodeBody(t, created)["data"].(map[string]any)
	require.True(t, ok)
	rentalID, ok := data["id"].(string)
	require.True(t, ok)

	returnBody := map[string]any{"return_date": "2026-01-06"}
	w := do(r, http.MethodPatch, "/api/v1/rentals/"+rentalID+"/return", adminToken(t), returnBody)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ReturnRental_CustomerForbidden(t *testing.T) {
	r := newTestRouter(t)
	returnBody := map[string]any{"return_date": "2026-01-06"}

	w := do(r, http.MethodPatch, "/api/v1/rentals/"+uuid.New().String()+"/return", customerToken(t), returnBody)

	require.Equal(t, http.StatusForbidden, w.Code)
}

// =============================================================================
// GET /api/v1/me/rentals -- any authenticated role
// =============================================================================

func TestRouter_MyRentals_CustomerAllowed(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodGet, "/api/v1/me/rentals", customerToken(t), nil)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_MyRentals_AdminAllowed(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodGet, "/api/v1/me/rentals", adminToken(t), nil)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_MyRentals_NoToken_Returns401(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodGet, "/api/v1/me/rentals", "", nil)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// =============================================================================
// GET /health -- public
// =============================================================================

func TestRouter_Health_IsPublicAndRequiresNoToken(t *testing.T) {
	r := newTestRouter(t)

	w := do(r, http.MethodGet, "/health", "", nil)

	require.Equal(t, http.StatusOK, w.Code)
}

// =============================================================================
// CORS
// =============================================================================

func TestRouter_CORSHeadersArePresent(t *testing.T) {
	r := newTestRouter(t)

	health := do(r, http.MethodGet, "/health", "", nil)
	require.Equal(t, http.StatusOK, health.Code)
	require.Equal(t, "http://localhost:3000", health.Header().Get("Access-Control-Allow-Origin"))

	// A CORS preflight to a protected path needs no token: the CORS
	// middleware is registered on the root engine and intercepts OPTIONS
	// before RequireAuth (which only guards the /api/v1 route group) ever
	// runs.
	preflight := do(r, http.MethodOptions, "/api/v1/rentals", "", nil)
	require.Equal(t, http.StatusNoContent, preflight.Code)
	require.Equal(t, "http://localhost:3000", preflight.Header().Get("Access-Control-Allow-Origin"))
}
