package service

import (
	"github.com/google/uuid"

	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

type UserService struct {
	users *repository.UserRepo
}

func NewUserService(users *repository.UserRepo) *UserService {
	return &UserService{users: users}
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
