package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireInternalKey protects internal-only endpoints (e.g. POST /auth/verify)
// meant to be called by other backend services, not end users. It checks a
// static shared secret in the X-Internal-Key header before any other work
// happens, so a wrong or missing key is rejected before the request body
// (e.g. a token to verify) is even looked at.
func RequireInternalKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Internal-Key") != expected {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"code": "FORBIDDEN", "message": "invalid internal key", "details": nil},
			})
			return
		}
		c.Next()
	}
}
