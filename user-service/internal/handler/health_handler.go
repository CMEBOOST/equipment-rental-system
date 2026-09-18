package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Health(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbStatus := "up"
		if db != nil {
			sqlDB, err := db.DB()
			if err != nil || sqlDB.Ping() != nil {
				dbStatus = "down"
			}
		}
		if dbStatus == "down" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"error":   gin.H{"code": "INTERNAL_ERROR", "message": "database unavailable", "details": nil},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    gin.H{"status": "ok", "service": "user-service", "db": dbStatus},
		})
	}
}
