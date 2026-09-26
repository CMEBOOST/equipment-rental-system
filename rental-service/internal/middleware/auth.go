package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireAuth decodes claims only. Kong has already verified the JWT signature
// and expiry for every protected route before forwarding the request here.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			unauthenticated(c)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			unauthenticated(c)
			return
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			unauthenticated(c)
			return
		}
		var claims struct {
			Subject string `json:"sub"`
			Role    string `json:"role"`
		}
		if err := json.Unmarshal(payload, &claims); err != nil {
			unauthenticated(c)
			return
		}
		userID, err := uuid.Parse(claims.Subject)
		if err != nil || (claims.Role != "admin" && claims.Role != "staff" && claims.Role != "customer") {
			unauthenticated(c)
			return
		}
		c.Set("user_id", userID)
		c.Set("role", claims.Role)
		c.Set("access_token", token)
		c.Next()
	}
}

func RequireRole(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		for _, permitted := range allowed {
			if role == permitted {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "FORBIDDEN", "message": "บทบาทไม่มีสิทธิ์เข้าถึง resource นี้", "details": nil}})
	}
}

func unauthenticated(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "ไม่มี token หรือ token ไม่ถูกต้อง", "details": nil}})
}
