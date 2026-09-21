package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/handler"
)

func TestHealth_DBUp_Returns200(t *testing.T) {
	// Create a real in-memory SQLite DB connection that stays open
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", handler.Health(db))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["success"])
	assert.Equal(t, "ok", body["data"].(map[string]any)["status"])
}

func TestHealth_DBDown_Returns503(t *testing.T) {
	// Create a real SQLite DB connection and close it to simulate DB-down
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Close the underlying SQL connection
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", handler.Health(db))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, false, body["success"])
	assert.Equal(t, "INTERNAL_ERROR", body["error"].(map[string]any)["code"])
	assert.Equal(t, "database unavailable", body["error"].(map[string]any)["message"])
}
