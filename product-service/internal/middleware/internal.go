package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequireInternalKey mirrors user-service/internal/middleware/internal.go:
// a constant-time comparison against a static shared secret, for endpoints
// meant to be called by other backend services rather than end users.
func RequireInternalKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Internal-Key")), []byte(expected)) != 1 {
			forbiddenInternalKey(c)
			return
		}
		c.Next()
	}
}

// RequireInternalKeyOrRole protects PATCH /products/{id}/status, the one
// endpoint CONTRACT.md §8.2 grants to three different kinds of caller:
// an admin/staff user (Bearer token, via Kong) *or* rental-service calling
// directly with X-Internal-Key (CONTRACT.md §7.3). A valid internal key
// short-circuits the JWT path entirely — there is no user to attach
// user_id/role for in a service-to-service call, so downstream handlers must
// not assume those context keys are set on this path.
func RequireInternalKeyOrRole(internalKey string, roles ...string) gin.HandlerFunc {
	roleGate := RequireRole(roles...)
	return func(c *gin.Context) {
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Internal-Key")), []byte(internalKey)) == 1 {
			c.Next()
			return
		}

		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			unauthenticated(c)
			return
		}
		claims, err := DecodeUnverified(token)
		if err != nil {
			unauthenticated(c)
			return
		}
		c.Set("user_id", claims.Subject)
		c.Set("role", claims.Role)
		c.Set("email", claims.Email)
		roleGate(c)
	}
}

func forbiddenInternalKey(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"success": false,
		"error":   gin.H{"code": "FORBIDDEN", "message": "invalid internal key", "details": nil},
	})
}
