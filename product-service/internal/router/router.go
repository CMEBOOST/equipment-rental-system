package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/config"
	"github.com/equipment-rental-system/product-service/internal/handler"
	"github.com/equipment-rental-system/product-service/internal/middleware"
	"github.com/equipment-rental-system/product-service/internal/repository"
	"github.com/equipment-rental-system/product-service/internal/service"
)

// New wires repositories -> services -> handlers and mounts every route in
// CONTRACT.md §8.2, with per-route auth mirroring §6.2's RBAC table.
func New(database *gorm.DB, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(cfg.CORSOrigin))
	r.GET("/health", handler.Health(database))

	categoryRepo := repository.NewCategoryRepo(database)
	productRepo := repository.NewProductRepo(database)
	productSvc := service.NewProductService(productRepo, categoryRepo)

	categoryHandler := handler.NewCategoryHandler(categoryRepo)
	productHandler := handler.NewProductHandler(productSvc)

	authMW := middleware.RequireAuth()

	v1 := r.Group("/api/v1")
	v1.GET("/categories", authMW, categoryHandler.List)

	v1.GET("/products", authMW, productHandler.List)
	// Admin/staff/customer (Bearer) OR rental-service (X-Internal-Key) — see
	// middleware.RequireInternalKeyOrAuth and CONTRACT.md §7.3/§8.2 (the
	// other endpoint, alongside PATCH .../status, that rental-service calls
	// directly with only an internal key, no user Bearer token).
	v1.GET("/products/:id", middleware.RequireInternalKeyOrAuth(cfg.InternalAPIKey), productHandler.Get)
	v1.POST("/products", authMW, middleware.RequireRole("admin", "staff"), productHandler.Create)
	v1.PUT("/products/:id", authMW, middleware.RequireRole("admin", "staff"), productHandler.Update)
	v1.DELETE("/products/:id", authMW, middleware.RequireRole("admin"), productHandler.Delete)
	// Admin/staff (Bearer) OR rental-service (X-Internal-Key) — see
	// middleware.RequireInternalKeyOrRole and CONTRACT.md §7.3/§8.2.
	v1.PATCH("/products/:id/status",
		middleware.RequireInternalKeyOrRole(cfg.InternalAPIKey, "admin", "staff"),
		productHandler.ChangeStatus)

	return r
}
