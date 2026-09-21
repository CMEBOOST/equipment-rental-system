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
)

// End-to-end through the HTTP layer: DELETE /users/{id} must free the
// account's email and username so POST /users can hand them to a new user.
// Before migration 000003 this returned 409 {"code":"CONFLICT"} -- an error
// code that is not even in the design doc's error table, describing a
// constraint the caller had no way to observe.
func TestAdminUserHandler_DeleteThenCreate_ReusesEmailAndUsername(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	userID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "reuse@example.com", "reuseuser", "hash", 3, true,
	).Error)

	// DELETE /api/v1/users/{id}
	wDel := httptest.NewRecorder()
	cDel, _ := gin.CreateTestContext(wDel)
	cDel.Request, _ = http.NewRequest("DELETE", "/api/v1/users/"+userID.String(), nil)
	cDel.Params = gin.Params{{Key: "id", Value: userID.String()}}
	cDel.Set("user_id", uuid.New().String())
	h.Delete(cDel)
	require.Equal(t, http.StatusOK, wDel.Code)

	// POST /api/v1/users with the deleted account's email AND username
	body, _ := json.Marshal(map[string]any{
		"email": "reuse@example.com", "username": "reuseuser",
		"password": "Passw0rd1", "role": "customer",
	})
	wNew := httptest.NewRecorder()
	cNew, _ := gin.CreateTestContext(wNew)
	cNew.Request, _ = http.NewRequest("POST", "/api/v1/users", bytes.NewReader(body))
	cNew.Request.Header.Set("Content-Type", "application/json")

	h.Create(cNew)

	require.Equal(t, http.StatusCreated, wNew.Code, "body: %s", wNew.Body.String())
	var resp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wNew.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, "reuse@example.com", resp.Data["email"])
	require.NotEqual(t, userID.String(), resp.Data["id"])
}
