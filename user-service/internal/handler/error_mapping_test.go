package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/repository"
)

// These tests cover the distinction the handlers used to lose: a real
// infrastructure failure must surface as 500 INTERNAL_ERROR, not as the
// 404 NOT_FOUND that every one of these handlers previously returned for
// *any* error. Dropping a table out from under the running handler is the
// simplest way to produce an error that is genuinely not
// gorm.ErrRecordNotFound.

func requireErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	require.False(t, resp.Success)
	require.Equal(t, want, resp.Error.Code)
}

func TestAdminUserHandler_Get_DBFailure_Returns500NotMisleading404(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "get500@example.com", "get500", "hash", 3, true,
	).Error)
	require.NoError(t, db.Exec(`DROP TABLE users`).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users/"+userID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}

	h.Get(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

func TestAdminUserHandler_ChangeRole_DBFailure_Returns500NotMisleading404(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "role500@example.com", "role500", "hash", 3, true,
	).Error)
	require.NoError(t, db.Exec(`DROP TABLE users`).Error)

	bodyBytes, _ := json.Marshal(map[string]any{"role": "staff"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+userID.String()+"/role", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	c.Set("user_id", uuid.New().String())

	h.ChangeRole(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

// This is the case the review called out specifically: UserService.ChangeStatus
// writes is_active and only then revokes the target's refresh tokens. When
// that second write fails, the account IS disabled -- reporting "user not
// found" actively contradicts the state the caller would read back. The
// operation is still non-transactional (accepted limitation), so the correct
// answer is 500.
func TestAdminUserHandler_ChangeStatus_RevokeFailure_Returns500_AndAccountIsDisabled(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "status500@example.com", "status500", "hash", 3, true,
	).Error)
	// Break only the *second* write of the sequence.
	require.NoError(t, db.Exec(`DROP TABLE refresh_tokens`).Error)

	bodyBytes, _ := json.Marshal(map[string]any{"is_active": false})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+userID.String()+"/status", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	c.Set("user_id", uuid.New().String())

	h.ChangeStatus(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")

	// The first write really did land -- which is exactly why a 404 here
	// would have been a lie.
	updated, err := repository.NewUserRepo(db).FindByID(userID)
	require.NoError(t, err)
	require.False(t, updated.IsActive)
}

func TestAdminUserHandler_Delete_RevokeFailure_Returns500NotMisleading404(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "del500@example.com", "del500", "hash", 3, true,
	).Error)
	require.NoError(t, db.Exec(`DROP TABLE refresh_tokens`).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("DELETE", "/api/v1/users/"+userID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	c.Set("user_id", uuid.New().String())

	h.Delete(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

func TestAdminUserHandler_LoginLogsForUser_DBFailure_Returns500NotMisleading404(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "logs500@example.com", "logs500", "hash", 3, true,
	).Error)
	require.NoError(t, db.Exec(`DROP TABLE users`).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users/"+userID.String()+"/login-logs", nil)
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}

	h.LoginLogsForUser(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

func TestUserHandler_Me_DBFailure_Returns500NotMisleading404(t *testing.T) {
	h, u, db := setupUserHandler(t)
	require.NoError(t, db.Exec(`DROP TABLE users`).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/me", nil)
	c.Set("user_id", u.ID.String())

	h.Me(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

// Guard rail: the 404 path must still be a 404 after the refactor.
func TestUserHandler_Me_UnknownUser_StillReturns404(t *testing.T) {
	h, _, _ := setupUserHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/me", nil)
	c.Set("user_id", uuid.New().String())

	h.Me(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "NOT_FOUND")
}
