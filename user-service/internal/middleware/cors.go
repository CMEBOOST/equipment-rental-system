package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS implements the browser-facing part of design doc §4/§8.6. The allowed
// origin is a single configured value rather than an echo of the request's
// Origin header, so a page on any other site cannot make a browser send a
// user's credentials here and read the response.
//
// Allow-Credentials is only sent for a concrete origin. The wildcard "*" and
// credentialed requests are mutually exclusive per the CORS spec, and sending
// both would make browsers reject the response outright.
//
// A preflight OPTIONS request is answered here and never reaches a handler.
// Gin runs global middleware for unmatched routes too, so this also covers
// preflights for paths that only register GET/POST/... methods.
// An empty origin falls back to "*" so a misconfigured deployment is not
// silently unreachable from every browser; config.Load() already supplies
// config.DefaultCORSOrigin when CORS_ORIGIN is unset.
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
