package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/equipment-rental-system/user-service/internal/service"
)

func RequireAuth(ts *service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			unauthenticated(c)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := ts.ParseAccessToken(token)
		if err != nil {
			unauthenticated(c)
			return
		}
		c.Set("user_id", claims.Sub)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func unauthenticated(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"error":   gin.H{"code": "UNAUTHENTICATED", "message": "ไม่มี token หรือ token ไม่ถูกต้อง", "details": nil},
	})
}
