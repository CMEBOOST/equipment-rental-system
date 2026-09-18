package service_test

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func setupAuthService(t *testing.T) *service.AuthService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}, &model.RefreshToken{}, &model.LoginLog{}))
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)

	cfg := &config.Config{BCryptCost: 4, JWTSecret: "test-secret-min-32-characters-ok", JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 168 * time.Hour}
	return service.NewAuthService(
		repository.NewUserRepo(db), repository.NewRoleRepo(db),
		repository.NewRefreshTokenRepo(db), repository.NewLoginLogRepo(db),
		service.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL), cfg,
	)
}

func TestAuthService_Register_DuplicateEmail_ReturnsErrEmailExists(t *testing.T) {
	svc := setupAuthService(t)
	req := dto.RegisterRequest{Email: "a@example.com", Username: "user1", Password: "Passw0rd1", FullName: "A"}

	_, err := svc.Register(req)
	require.NoError(t, err)

	req.Username = "user2"
	_, err = svc.Register(req)
	require.ErrorIs(t, err, service.ErrEmailExists)
}

func TestAuthService_Register_AssignsCustomerRole(t *testing.T) {
	svc := setupAuthService(t)
	u, err := svc.Register(dto.RegisterRequest{Email: "b@example.com", Username: "userb", Password: "Passw0rd1"})
	require.NoError(t, err)
	require.Equal(t, int16(3), u.RoleID)
}
