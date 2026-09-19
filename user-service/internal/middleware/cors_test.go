package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/middleware"
)

func corsEngine(origin string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CORS(origin))
	r.GET("/thing", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func TestCORS_AddsConfiguredOriginToResponse(t *testing.T) {
	r := corsEngine("http://localhost:3000")

	req, _ := http.NewRequest("GET", "/thing", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// The configured origin is returned regardless of what the caller claims,
	// so another site's page cannot read this response.
	require.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	require.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORS_PreflightIsAnsweredWithoutReachingHandler(t *testing.T) {
	r := corsEngine("http://localhost:3000")

	req, _ := http.NewRequest("OPTIONS", "/thing", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, w.Body.String())
}

func TestCORS_WildcardOriginOmitsCredentials(t *testing.T) {
	r := corsEngine("*")

	req, _ := http.NewRequest("GET", "/thing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	// "*" and credentials are mutually exclusive; sending both would make
	// browsers reject every response.
	require.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}
