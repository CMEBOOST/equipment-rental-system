package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireInternalKey protects internal-only endpoints (e.g. POST /auth/verify)
// meant to be called by other backend services, not end users. It checks a
// static shared secret in the X-Internal-Key header before any other work
// happens, so a wrong or missing key is rejected before the request body
// (e.g. a token to verify) is even looked at.
//
// The comparison uses crypto/subtle.ConstantTimeCompare so the time taken to
// reject a wrong key does not depend on how many leading bytes were correct,
// which would otherwise let a caller recover the shared secret byte by byte.
//
// An empty `expected` cannot reach here in a running service: config.Load()
// refuses to start when INTERNAL_API_KEY is unset, precisely because an empty
// expected key would match a request that omits the header entirely.
func RequireInternalKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Internal-Key")), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"code": "FORBIDDEN", "message": "invalid internal key", "details": nil},
			})
			return
		}
		c.Next()
	}
}
