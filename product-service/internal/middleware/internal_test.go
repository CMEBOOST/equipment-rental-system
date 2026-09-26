package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/product-service/internal/middleware"
)

func newStatusRouter(internalKey string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PATCH("/products/:id/status",
		middleware.RequireInternalKeyOrRole(internalKey, "admin", "staff"),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func TestRequireInternalKeyOrRole_ValidInternalKey_BypassesJWTEntirely(t *testing.T) {
	r := newStatusRouter("secret-key")
	req := httptest.NewRequest(http.MethodPatch, "/products/1/status", nil)
	req.Header.Set("X-Internal-Key", "secret-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireInternalKeyOrRole_WrongInternalKey_FallsThroughToUnauthenticated(t *testing.T) {
	r := newStatusRouter("secret-key")
	req := httptest.NewRequest(http.MethodPatch, "/products/1/status", nil)
	req.Header.Set("X-Internal-Key", "wrong-key")
	// no Authorization header either — must not be treated as authenticated
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireInternalKeyOrRole_NoInternalKey_ValidAdminToken_Allowed(t *testing.T) {
	r := newStatusRouter("secret-key")
	token := signRoleToken(t, "admin")
	req := httptest.NewRequest(http.MethodPatch, "/products/1/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireInternalKeyOrRole_NoInternalKey_CustomerToken_Forbidden(t *testing.T) {
	r := newStatusRouter("secret-key")
	token := signRoleToken(t, "customer")
	req := httptest.NewRequest(http.MethodPatch, "/products/1/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireInternalKeyOrRole_NoKeyNoToken_Unauthenticated(t *testing.T) {
	r := newStatusRouter("secret-key")
	req := httptest.NewRequest(http.MethodPatch, "/products/1/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func newProductGetRouter(internalKey string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/products/:id",
		middleware.RequireInternalKeyOrAuth(internalKey),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func TestRequireInternalKeyOrAuth_ValidInternalKey_BypassesJWTEntirely(t *testing.T) {
	r := newProductGetRouter("secret-key")
	req := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	req.Header.Set("X-Internal-Key", "secret-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireInternalKeyOrAuth_WrongInternalKey_FallsThroughToUnauthenticated(t *testing.T) {
	r := newProductGetRouter("secret-key")
	req := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	req.Header.Set("X-Internal-Key", "wrong-key")
	// no Authorization header either — must not be treated as authenticated
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireInternalKeyOrAuth_NoInternalKey_AnyAuthenticatedRole_Allowed(t *testing.T) {
	r := newProductGetRouter("secret-key")
	token := signRoleToken(t, "customer") // any role, unlike RequireInternalKeyOrRole
	req := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireInternalKeyOrAuth_NoKeyNoToken_Unauthenticated(t *testing.T) {
	r := newProductGetRouter("secret-key")
	req := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func signRoleToken(t *testing.T, role string) string {
	t.Helper()
	claims := middleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte("irrelevant-product-service-does-not-verify-this"))
	require.NoError(t, err)
	return s
}
