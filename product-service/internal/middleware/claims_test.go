package middleware_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/product-service/internal/middleware"
)

// signToken builds a real HS256 token so DecodeUnverified is exercised
// against something structurally identical to what user-service issues
// (CONTRACT.md §5.2) — the point being proven is that product-service can
// read the claims WITHOUT the signing secret, not that the library can sign.
func signToken(t *testing.T, secret string, claims middleware.Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

func TestDecodeUnverified_ReadsClaimsWithoutTheSigningSecret(t *testing.T) {
	claims := middleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "11111111-1111-1111-1111-111111111111",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email: "somchai@example.com",
		Role:  "admin",
	}
	// Signed with a secret product-service never holds — DecodeUnverified
	// must still succeed, because it never checks the signature at all.
	token := signToken(t, "a-secret-only-user-service-has", claims)

	got, err := middleware.DecodeUnverified(token)
	require.NoError(t, err)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", got.Subject)
	assert.Equal(t, "admin", got.Role)
	assert.Equal(t, "somchai@example.com", got.Email)
}

func TestDecodeUnverified_RejectsExpiredToken(t *testing.T) {
	claims := middleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)), // already expired
		},
		Role: "customer",
	}
	token := signToken(t, "any-secret", claims)

	_, err := middleware.DecodeUnverified(token)
	assert.Error(t, err)
}

func TestDecodeUnverified_RejectsMalformedToken(t *testing.T) {
	_, err := middleware.DecodeUnverified("not-a-jwt-at-all")
	assert.Error(t, err)
}

func TestDecodeUnverified_AcceptsTokenSignedWithWrongAlgorithmClaimsOnly(t *testing.T) {
	// Belt-and-suspenders: even a token whose alg differs from HS256 still
	// decodes here, because ParseUnverified never looks at alg to verify a
	// signature — it only reads header+payload. This is the documented,
	// deliberate behavior (CONTRACT.md §5.3): Kong is the only signature
	// verifier in this system.
	claims := middleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-2",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: "staff",
	}
	token := signToken(t, "irrelevant", claims)

	got, err := middleware.DecodeUnverified(token)
	require.NoError(t, err)
	assert.Equal(t, "staff", got.Role)
}
