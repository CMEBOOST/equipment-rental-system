package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/middleware"
)

func TestRequireInternalKey_WrongKey_Returns403(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal", middleware.RequireInternalKey("correct-key"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/internal", nil)
	req.Header.Set("X-Internal-Key", "wrong-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireInternalKey_MissingKey_Returns403(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal", middleware.RequireInternalKey("correct-key"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/internal", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireInternalKey_CorrectKey_Passes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal", middleware.RequireInternalKey("correct-key"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/internal", nil)
	req.Header.Set("X-Internal-Key", "correct-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}
