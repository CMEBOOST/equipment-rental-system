package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Postgres declares email VARCHAR(255), username VARCHAR(50),
// full_name VARCHAR(120) and phone VARCHAR(20) (migration 000001), but only
// username carried a max= binding tag. An over-length value therefore
// reached the driver and came back as a raw error, which the handlers'
// default branch reported as 500 INTERNAL_ERROR -- a client mistake blamed
// on the server. These tests assert the request is rejected as 400
// VALIDATION_ERROR by binding, before any DB call.
//
// The SQLite harness does not enforce VARCHAR widths, which is exactly why
// this class of bug was invisible: only a request-level test can prove the
// value never gets that far.

var (
	tooLongName  = strings.Repeat("n", 121)                    // full_name is VARCHAR(120)
	tooLongPhone = strings.Repeat("9", 21)                     // phone is VARCHAR(20)
	tooLongEmail = strings.Repeat("a", 250) + "@example.com"   // 262 chars, still well-formed
	tooLongPass  = strings.Repeat("aB1", 25)                   // 75 bytes; bcrypt's limit is 72
)

func requireValidationError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	requireErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

// ---------- RegisterRequest ----------

func TestDTO_Register_OverLongFullName_Returns400(t *testing.T) {
	h, _, db := setupAuthHandler(t)

	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "len@example.com", "username": "lenuser", "password": "Passw0rd1",
		"full_name": tooLongName,
	})

	requireValidationError(t, w)
	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM users`).Scan(&n).Error)
	require.Zero(t, n, "the request must be rejected before it reaches the database")
}

func TestDTO_Register_OverLongPhone_Returns400(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "len2@example.com", "username": "len2user", "password": "Passw0rd1",
		"phone": tooLongPhone,
	})

	requireValidationError(t, w)
}

func TestDTO_Register_OverLongEmail_Returns400(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": tooLongEmail, "username": "len3user", "password": "Passw0rd1",
	})

	requireValidationError(t, w)
}

// bcrypt rejects inputs over 72 bytes, so an unbounded password would fail
// inside GenerateFromPassword and surface as a 500.
func TestDTO_Register_OverLongPassword_Returns400(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Register, "POST", "/api/v1/auth/register", map[string]any{
		"email": "len4@example.com", "username": "len4user", "password": tooLongPass,
	})

	requireValidationError(t, w)
}

// ---------- LoginRequest ----------

// login_logs.email_attempted is VARCHAR(255) and every attempt is recorded
// there, so an unbounded login email would silently break the audit insert.
func TestDTO_Login_OverLongEmail_Returns400(t *testing.T) {
	h, _, _ := setupAuthHandler(t)

	w := doJSON(h.Login, "POST", "/api/v1/auth/login", map[string]any{
		"email": tooLongEmail, "password": "Passw0rd1",
	})

	requireValidationError(t, w)
}

// ---------- CreateUserRequest ----------

func postAdminJSON(h interface{ Create(*gin.Context) }, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/api/v1/users", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Create(c)
	return w
}

func TestDTO_CreateUser_OverLongFullName_Returns400(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	w := postAdminJSON(h, map[string]any{
		"email": "clen@example.com", "username": "clenuser", "password": "Passw0rd1",
		"role": "staff", "full_name": tooLongName,
	})

	requireValidationError(t, w)
}

func TestDTO_CreateUser_OverLongEmail_Returns400(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	w := postAdminJSON(h, map[string]any{
		"email": tooLongEmail, "username": "clen2user", "password": "Passw0rd1", "role": "staff",
	})

	requireValidationError(t, w)
}

func TestDTO_CreateUser_OverLongPhone_Returns400(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	w := postAdminJSON(h, map[string]any{
		"email": "clen3@example.com", "username": "clen3user", "password": "Passw0rd1",
		"role": "staff", "phone": tooLongPhone,
	})

	requireValidationError(t, w)
}

// ---------- UpdateUserRequest (PUT /users/{id}) ----------

func putUser(t *testing.T, h interface{ Update(*gin.Context) }, id uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/users/"+id.String(), bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: id.String()}}
	h.Update(c)
	return w
}

func TestDTO_UpdateUser_MalformedEmail_Returns400(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, phone, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), "upd@example.com", "upduser", "hash", "Upd", "", 3, true,
	).Error)

	// UpdateUserRequest previously had no validation at all on Email.
	w := putUser(t, h, id, map[string]any{"email": "definitely-not-an-email"})

	requireValidationError(t, w)

	var stored string
	require.NoError(t, db.Raw(`SELECT email FROM users WHERE id = ?`, id.String()).Scan(&stored).Error)
	require.Equal(t, "upd@example.com", stored, "the malformed address must not be stored")
}

func TestDTO_UpdateUser_OverLongFullName_Returns400(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, phone, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), "upd2@example.com", "upd2user", "hash", "Upd", "", 3, true,
	).Error)

	w := putUser(t, h, id, map[string]any{"full_name": tooLongName})

	requireValidationError(t, w)
}

func TestDTO_UpdateUser_OverLongPhone_Returns400(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, phone, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), "upd3@example.com", "upd3user", "hash", "Upd", "", 3, true,
	).Error)

	w := putUser(t, h, id, map[string]any{"phone": tooLongPhone})

	requireValidationError(t, w)
}

// A well-formed update must still pass, i.e. the new tags do not reject
// ordinary requests or partial (omitted-field) updates.
func TestDTO_UpdateUser_ValidPartialUpdate_StillSucceeds(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, full_name, phone, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), "upd4@example.com", "upd4user", "hash", "Upd", "", 3, true,
	).Error)

	w := putUser(t, h, id, map[string]any{"full_name": "Perfectly Fine Name"})

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

// ---------- UpdateProfileRequest (PUT /me) ----------

func TestDTO_UpdateMe_OverLongFullName_Returns400(t *testing.T) {
	h, u, _ := setupUserHandler(t)

	b, _ := json.Marshal(map[string]any{"full_name": tooLongName})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/me", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", u.ID.String())

	h.UpdateMe(c)

	requireValidationError(t, w)
}

func TestDTO_UpdateMe_OverLongPhone_Returns400(t *testing.T) {
	h, u, _ := setupUserHandler(t)

	b, _ := json.Marshal(map[string]any{"phone": tooLongPhone})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/me", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", u.ID.String())

	h.UpdateMe(c)

	requireValidationError(t, w)
}

// ---------- ChangePasswordRequest ----------

func TestDTO_ChangePassword_OverLongNewPassword_Returns400(t *testing.T) {
	h, u, _ := setupUserHandler(t)

	b, _ := json.Marshal(map[string]any{"current_password": "whatever", "new_password": tooLongPass})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PUT", "/api/v1/me/password", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", u.ID.String())

	h.ChangePassword(c)

	requireValidationError(t, w)
}
