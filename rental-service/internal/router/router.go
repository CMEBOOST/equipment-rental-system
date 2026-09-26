package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/rental-service/internal/client"
	"github.com/equipment-rental-system/rental-service/internal/config"
	"github.com/equipment-rental-system/rental-service/internal/handler"
	"github.com/equipment-rental-system/rental-service/internal/middleware"
	"github.com/equipment-rental-system/rental-service/internal/repository"
	"github.com/equipment-rental-system/rental-service/internal/service"
)

func New(database *gorm.DB, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(cfg.CORSOrigin))
	r.GET("/health", handler.Health(database))
	repo := repository.NewRentalRepository(database)
	svc := service.NewRentalService(repo, client.NewProductClient(cfg.ProductServiceURL, cfg.InternalAPIKey), client.NewUserClient(cfg.UserServiceURL, cfg.InternalAPIKey))
	h := handler.NewRentalHandler(svc)
	v1 := r.Group("/api/v1")
	v1.Use(middleware.RequireAuth())
	v1.GET("/rentals", middleware.RequireRole("admin", "staff"), h.List)
	v1.GET("/rentals/:id", h.Get)
	v1.POST("/rentals", middleware.RequireRole("admin", "staff"), h.Create)
	v1.POST("/rentals/request", middleware.RequireRole("customer"), h.Request)
	v1.PATCH("/rentals/:id/approve", middleware.RequireRole("admin", "staff"), h.Approve)
	v1.PATCH("/rentals/:id/return", middleware.RequireRole("admin", "staff"), h.Return)
	v1.GET("/me/rentals", h.MyList)
	return r
}
