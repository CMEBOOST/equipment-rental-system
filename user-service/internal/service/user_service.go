package service

import (
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

type UserService struct {
	users         *repository.UserRepo
	refreshTokens *repository.RefreshTokenRepo
	cfg           *config.Config
}

func NewUserService(users *repository.UserRepo, refreshTokens *repository.RefreshTokenRepo, cfg *config.Config) *UserService {
	return &UserService{users: users, refreshTokens: refreshTokens, cfg: cfg}
}

func (s *UserService) GetProfile(userID uuid.UUID) (*model.User, error) {
	return s.users.FindByID(userID)
}

func (s *UserService) UpdateProfile(userID uuid.UUID, req dto.UpdateProfileRequest) (*model.User, error) {
	u, err := s.users.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if req.FullName != nil {
		u.FullName = *req.FullName
	}
	if req.Phone != nil {
		u.Phone = *req.Phone
	}
	if err := s.users.Update(u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserService) ChangePassword(userID uuid.UUID, req dto.ChangePasswordRequest) error {
	u, err := s.users.FindByID(userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.CurrentPassword)) != nil {
		return ErrInvalidCredentials
	}
	if !isStrongPassword(req.NewPassword) {
		return ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.cfg.BCryptCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	if err := s.users.Update(u); err != nil {
		return err
	}
	return s.refreshTokens.RevokeAllForUser(userID)
}
