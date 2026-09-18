package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/handler"
	"github.com/equipment-rental-system/user-service/internal/middleware"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func New(database *gorm.DB, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", handler.Health(database))

	userRepo := repository.NewUserRepo(database)
	roleRepo := repository.NewRoleRepo(database)
	refreshRepo := repository.NewRefreshTokenRepo(database)
	logRepo := repository.NewLoginLogRepo(database)
	tokenSvc := service.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL)
	authSvc := service.NewAuthService(userRepo, roleRepo, refreshRepo, logRepo, tokenSvc, cfg)
	authHandler := handler.NewAuthHandler(authSvc)
	authMW := middleware.RequireAuth(tokenSvc)

	userSvc := service.NewUserService(userRepo, refreshRepo, cfg, logRepo)
	userHandler := handler.NewUserHandler(userSvc)
	adminUserHandler := handler.NewAdminUserHandler(userSvc, roleRepo, cfg.BCryptCost)
	roleHandler := handler.NewRoleHandler(roleRepo)

	v1 := r.Group("/api/v1")
	v1.POST("/auth/register", authHandler.Register)
	v1.POST("/auth/login", authHandler.Login)
	v1.POST("/auth/refresh", authHandler.Refresh)
	v1.POST("/auth/logout", authMW, authHandler.Logout)
	v1.POST("/auth/verify", middleware.RequireInternalKey(cfg.InternalAPIKey), authHandler.Verify)
	v1.GET("/me", authMW, userHandler.Me)
	v1.PUT("/me", authMW, userHandler.UpdateMe)
	v1.PUT("/me/password", authMW, userHandler.ChangePassword)
	v1.GET("/me/login-logs", authMW, userHandler.MyLoginLogs)
	v1.GET("/me/sessions", authMW, userHandler.MySessions)
	v1.GET("/users", authMW, middleware.RequireRole("admin"), adminUserHandler.List)
	v1.POST("/users", authMW, middleware.RequireRole("admin"), adminUserHandler.Create)
	v1.GET("/users/:id", authMW, middleware.RequireRole("admin", "staff"), adminUserHandler.Get)
	v1.PUT("/users/:id", authMW, middleware.RequireRole("admin"), adminUserHandler.Update)
	v1.DELETE("/users/:id", authMW, middleware.RequireRole("admin"), adminUserHandler.Delete)
	v1.PATCH("/users/:id/role", authMW, middleware.RequireRole("admin"), adminUserHandler.ChangeRole)
	v1.PATCH("/users/:id/status", authMW, middleware.RequireRole("admin"), adminUserHandler.ChangeStatus)
	v1.GET("/users/:id/login-logs", authMW, middleware.RequireRole("admin"), adminUserHandler.LoginLogsForUser)
	v1.GET("/roles", authMW, middleware.RequireRole("admin", "staff"), roleHandler.List)
	return r
}
