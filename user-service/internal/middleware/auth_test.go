package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/middleware"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func TestRequireAuth_NoToken_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ts := service.NewTokenService("test-secret-min-32-characters-ok", 15*time.Minute)
	r := gin.New()
	r.GET("/protected", middleware.RequireAuth(ts), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_ValidToken_SetsUserIDAndRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ts := service.NewTokenService("test-secret-min-32-characters-ok", 15*time.Minute)
	u := model.User{ID: uuid.New(), Role: model.Role{Name: "staff"}}
	token, _, err := ts.GenerateAccessToken(u)
	require.NoError(t, err)

	var gotRole any
	r := gin.New()
	r.GET("/protected", middleware.RequireAuth(ts), func(c *gin.Context) {
		gotRole, _ = c.Get("role")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "staff", gotRole)
}
