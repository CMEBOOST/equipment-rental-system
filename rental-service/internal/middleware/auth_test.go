package middleware_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/rental-service/internal/middleware"
)

// fakeToken builds a base64url three-segment string shaped like a JWT.
// RequireAuth only decodes the payload segment (Kong has already verified
// the real signature/expiry before forwarding here), so the header and
// signature segments never need to be cryptographically valid — they only
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

func runRequireAuth(t *testing.T, authHeader string) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	c.Request = req
	middleware.RequireAuth()(c)
	return w, c
}

// runRequireRole invokes RequireRole(allowed...) against a fresh context that
// already has "role" set to roleInContext (skipped when roleInContext is
// empty, to exercise the "no role in context" case).
func runRequireRole(t *testing.T, roleInContext string, allowed ...string) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	c.Request = req
	if roleInContext != "" {
		c.Set("role", roleInContext)
	}
	middleware.RequireRole(allowed...)(c)
	return w, c
}

func TestRequireAuth_MissingAuthorizationHeader_Returns401(t *testing.T) {
	w, c := runRequireAuth(t, "")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.True(t, c.IsAborted())
}

func TestRequireAuth_HeaderMissingBearerPrefix_Returns401(t *testing.T) {
	w, c := runRequireAuth(t, "Basic abc123")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.True(t, c.IsAborted())
}

func TestRequireAuth_TokenNotThreeSegments_Returns401(t *testing.T) {
	w, c := runRequireAuth(t, "Bearer only.two")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.True(t, c.IsAborted())
}

func TestRequireAuth_PayloadSegmentNotValidBase64_Returns401(t *testing.T) {
	w, c := runRequireAuth(t, "Bearer header.not-valid-base64!!!.signature")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.True(t, c.IsAborted())
}

func TestRequireAuth_PayloadSegmentNotValidJSON_Returns401(t *testing.T) {
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	w, c := runRequireAuth(t, "Bearer header."+notJSON+".signature")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.True(t, c.IsAborted())
}

func TestRequireAuth_ValidAdminClaims_SetsContextAndDoesNotAbort(t *testing.T) {
	userID := uuid.New()
	token := fakeToken(t, map[string]any{"sub": userID.String(), "role": "admin"})

	w, c := runRequireAuth(t, "Bearer "+token)

	require.False(t, c.IsAborted())
	require.Equal(t, http.StatusOK, w.Code)
	gotUserID, exists := c.Get("user_id")
	require.True(t, exists)
	require.Equal(t, userID, gotUserID)
	gotRole, exists := c.Get("role")
	require.True(t, exists)
	require.Equal(t, "admin", gotRole)
}

func TestRequireAuth_ValidStaffClaims_SetsContextAndDoesNotAbort(t *testing.T) {
	userID := uuid.New()
	token := fakeToken(t, map[string]any{"sub": userID.String(), "role": "staff"})

	w, c := runRequireAuth(t, "Bearer "+token)

	require.False(t, c.IsAborted())
	require.Equal(t, http.StatusOK, w.Code)
	gotRole, exists := c.Get("role")
	require.True(t, exists)
	require.Equal(t, "staff", gotRole)
}

func TestRequireAuth_ValidCustomerClaims_SetsContextAndDoesNotAbort(t *testing.T) {
	userID := uuid.New()
	token := fakeToken(t, map[string]any{"sub": userID.String(), "role": "customer"})

	w, c := runRequireAuth(t, "Bearer "+token)

	require.False(t, c.IsAborted())
	require.Equal(t, http.StatusOK, w.Code)
	gotRole, exists := c.Get("role")
	require.True(t, exists)
	require.Equal(t, "customer", gotRole)
}

func TestRequireAuth_UnknownRoleValue_Returns401(t *testing.T) {
	token := fakeToken(t, map[string]any{"sub": uuid.New().String(), "role": "superuser"})

	w, c := runRequireAuth(t, "Bearer "+token)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.True(t, c.IsAborted())
}

func TestRequireAuth_NonUUIDSubject_Returns401(t *testing.T) {
	token := fakeToken(t, map[string]any{"sub": "not-a-uuid", "role": "admin"})

	w, c := runRequireAuth(t, "Bearer "+token)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.True(t, c.IsAborted())
}

func TestRequireAuth_SetsAccessTokenInContext(t *testing.T) {
	token := fakeToken(t, map[string]any{"sub": uuid.New().String(), "role": "admin"})

	_, c := runRequireAuth(t, "Bearer "+token)

	gotToken, exists := c.Get("access_token")
	require.True(t, exists)
	require.Equal(t, token, gotToken)
}

func TestRequireRole_AllowedRole_DoesNotAbort(t *testing.T) {
	w, c := runRequireRole(t, "admin", "admin", "staff")

	require.False(t, c.IsAborted())
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_DisallowedRole_Returns403(t *testing.T) {
	w, c := runRequireRole(t, "customer", "admin", "staff")

	require.Equal(t, http.StatusForbidden, w.Code)
	require.True(t, c.IsAborted())
}

func TestRequireRole_NoRoleInContext_Returns403(t *testing.T) {
	w, c := runRequireRole(t, "", "admin")

	require.Equal(t, http.StatusForbidden, w.Code)
	require.True(t, c.IsAborted())
}
