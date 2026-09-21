package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

// Design doc §7.9/§7.17 specify a success=true|false filter on both
// login-log endpoints. Neither the repository nor either handler
// implemented it, so the parameter was silently ignored and a caller
// auditing failed sign-in attempts got the whole history back.

type logListResponse struct {
	Success bool             `json:"success"`
	Data    []map[string]any `json:"data"`
	Meta    map[string]any   `json:"meta"`
}

// seedLoginLogs writes 3 successful and 2 failed attempts for userID.
func seedLoginLogs(t *testing.T, db *gorm.DB, userID uuid.UUID) {
	t.Helper()
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&model.LoginLog{
			UserID: &userID, EmailAttempted: "who@example.com", Success: true, IPAddress: "1.1.1.1",
		}).Error)
	}
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&model.LoginLog{
			UserID: &userID, EmailAttempted: "who@example.com", Success: false, IPAddress: "1.1.1.2",
		}).Error)
	}
}

func decodeLogList(t *testing.T, w *httptest.ResponseRecorder) logListResponse {
	t.Helper()
	var body logListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success, "body: %s", w.Body.String())
	return body
}

// ---------- GET /me/login-logs ----------

func myLogs(t *testing.T, h interface{ MyLoginLogs(*gin.Context) }, userID uuid.UUID, query string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/me/login-logs"+query, nil)
	c.Set("user_id", userID.String())
	h.MyLoginLogs(c)
	return w
}

func TestMyLoginLogs_SuccessFilterNarrowsResults(t *testing.T) {
	h, u, db := setupUserHandler(t)
	seedLoginLogs(t, db, u.ID)

	all := decodeLogList(t, myLogs(t, h, u.ID, ""))
	require.Len(t, all.Data, 5, "no filter must return both outcomes")
	require.Equal(t, float64(5), all.Meta["total"])

	failed := myLogs(t, h, u.ID, "?success=false")
	require.Equal(t, http.StatusOK, failed.Code)
	body := decodeLogList(t, failed)
	require.Len(t, body.Data, 2)
	// meta.total must describe the filtered set, not the whole history.
	require.Equal(t, float64(2), body.Meta["total"])
	for _, item := range body.Data {
		require.Equal(t, false, item["success"])
	}

	ok := decodeLogList(t, myLogs(t, h, u.ID, "?success=true"))
	require.Len(t, ok.Data, 3)
	require.Equal(t, float64(3), ok.Meta["total"])
	for _, item := range ok.Data {
		require.Equal(t, true, item["success"])
	}
}

// An empty value is treated as "not supplied" rather than as an error, so
// ?success= from a form that submitted a blank field still works.
func TestMyLoginLogs_EmptySuccessValue_IsNoFilter(t *testing.T) {
	h, u, db := setupUserHandler(t)
	seedLoginLogs(t, db, u.ID)

	body := decodeLogList(t, myLogs(t, h, u.ID, "?success="))

	require.Len(t, body.Data, 5)
}

func TestMyLoginLogs_InvalidSuccessValue_Returns400(t *testing.T) {
	h, u, db := setupUserHandler(t)
	seedLoginLogs(t, db, u.ID)

	w := myLogs(t, h, u.ID, "?success=maybe")

	require.Equal(t, http.StatusBadRequest, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

// ---------- GET /users/{id}/login-logs ----------

func TestAdminLoginLogsForUser_SuccessFilterNarrowsResults(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "audit@example.com", "audituser", "hash", 3, true,
	).Error)
	seedLoginLogs(t, db, userID)

	req := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/api/v1/users/"+userID.String()+"/login-logs"+query, nil)
		c.Params = gin.Params{{Key: "id", Value: userID.String()}}
		h.LoginLogsForUser(c)
		return w
	}

	all := decodeLogList(t, req(""))
	require.Len(t, all.Data, 5)

	failed := decodeLogList(t, req("?success=false"))
	require.Len(t, failed.Data, 2)
	require.Equal(t, float64(2), failed.Meta["total"])
	for _, item := range failed.Data {
		require.Equal(t, false, item["success"])
	}

	bad := req("?success=1")
	require.Equal(t, http.StatusBadRequest, bad.Code)
	requireErrorCode(t, bad.Body.Bytes(), "VALIDATION_ERROR")
}

// ---------- GET /users?is_active= ----------

// The pre-existing is_active filter used `b := v == "true"`, which turned
// every typo into a filter for false. It now validates the same way.
func TestAdminList_InvalidIsActiveValue_Returns400(t *testing.T) {
	h, _ := setupAdminUserHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users?is_active=yes", nil)

	h.List(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

func TestAdminList_IsActiveFilterStillNarrowsResults(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	for i, active := range []bool{true, true, false} {
		require.NoError(t, db.Exec(
			`INSERT INTO users (id, email, username, password_hash, role_id, is_active, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), "u"+string(rune('a'+i))+"@example.com", "user"+string(rune('a'+i)), "hash", 3, active, "2026-01-0"+string(rune('1'+i)),
		).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/users?is_active=false", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	body := decodeLogList(t, w)
	require.Len(t, body.Data, 1)
	require.Equal(t, false, body.Data[0]["is_active"])
}
