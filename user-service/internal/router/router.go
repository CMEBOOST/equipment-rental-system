package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/handler"
)

func New(database *gorm.DB, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", handler.Health(database))
	// v1 group + auth/user/role routes are added by later tasks
	return r
}
