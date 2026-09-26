package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireRole mirrors user-service/internal/middleware/rbac.go byte-for-byte:
// same response shape, same "unknown role -> 403" behavior.
func RequireRole(allowed ...string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleStr, _ := role.(string)
		if _, ok := set[roleStr]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"code": "FORBIDDEN", "message": "บทบาทไม่มีสิทธิ์เข้าถึง resource นี้", "details": nil},
			})
			return
		}
		c.Next()
	}
}
