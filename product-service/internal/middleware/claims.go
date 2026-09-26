// Package middleware implements product-service's Gin middleware chain.
//
// Unlike user-service (which issues and verifies tokens with JWT_SECRET),
// product-service never holds that secret. Per CONTRACT.md §5.3, Kong's jwt
// plugin verifies the token's signature and expiry before a request reaches
// this service at all; product-service only decodes the already-trusted
// payload to read `sub`/`role`/`email` for its own RBAC decisions.
//
// The corollary (also spelled out in §5.3): hitting this service directly on
// :8082, bypassing Kong, skips that verification entirely. That path exists
// for local dev/debugging (§9.4), not for anything that must reject a forged
// or expired token — those checks are only real when exercised through Kong
// on :8000.
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims mirrors the access-token payload user-service signs
// (CONTRACT.md §5.2).
type Claims struct {
	jwt.RegisteredClaims
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// DecodeUnverified base64-decodes a JWT's payload segment and unmarshals its
// claims WITHOUT checking the signature — ParseUnverified never touches the
// signature segment at all, so no secret is needed or possible here.
//
// It still rejects an expired `exp` locally. That is not a security check
// (a forged token could just as easily forge a future exp; only Kong's
// signature verification actually prevents that) — it only stops this
// service from acting on a token its own legitimate holder already knows is
// stale on the direct-to-service debug path where Kong is not in front of
// it.
func DecodeUnverified(tokenString string) (*Claims, error) {
	claims := &Claims{}
	if _, _, err := jwt.NewParser().ParseUnverified(tokenString, claims); err != nil {
		return nil, err
	}
	if exp, err := claims.GetExpirationTime(); err == nil && exp != nil && exp.Before(time.Now()) {
		return nil, jwt.ErrTokenExpired
	}
	return claims, nil
}

// RequireAuth reads the Bearer token, decodes its claims (see
// DecodeUnverified), and stores user_id/role/email in the Gin context for
// downstream handlers and RequireRole.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
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
		c.Next()
	}
}

func unauthenticated(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"error":   gin.H{"code": "UNAUTHENTICATED", "message": "ไม่มี token หรือ token ไม่ถูกต้อง", "details": nil},
	})
}
