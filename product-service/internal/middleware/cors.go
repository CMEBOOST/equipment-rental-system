package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS mirrors user-service/internal/middleware/cors.go — same reasoning
// applies here verbatim, see that file's comment for the full rationale
// (single configured origin vs echoing Origin, Allow-Credentials only for a
// concrete origin, preflight handled here).
func CORS(origin string) gin.HandlerFunc {
	if origin == "" {
		origin = "*"
	}
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Internal-Key")
		c.Header("Access-Control-Max-Age", "600")
		if origin != "*" {
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
