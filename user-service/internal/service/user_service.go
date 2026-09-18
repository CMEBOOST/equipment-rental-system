package service

import (
	"errors"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

type UserService struct {
	users         *repository.UserRepo
	refreshTokens *repository.RefreshTokenRepo
	cfg           *config.Config
	loginLogs     *repository.LoginLogRepo
}

func NewUserService(users *repository.UserRepo, refreshTokens *repository.RefreshTokenRepo, cfg *config.Config, loginLogs *repository.LoginLogRepo) *UserService {
	return &UserService{users: users, refreshTokens: refreshTokens, cfg: cfg, loginLogs: loginLogs}
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

func (s *UserService) LoginLogsForUser(userID uuid.UUID, page, limit int) ([]model.LoginLog, int64, error) {
	return s.loginLogs.ListForUser(userID, page, limit)
}

func (s *UserService) ActiveSessions(userID uuid.UUID) ([]model.RefreshToken, error) {
	return s.refreshTokens.ListActiveForUser(userID)
}

// ListUsers is a thin pass-through to UserRepo.List, which already applies
// the sort/order allowlist (SQL-injection guard from Task 4) and clamps
// pagination.
func (s *UserService) ListUsers(f repository.UserFilter) ([]model.User, int64, error) {
	return s.users.List(f)
}

// CreateUser is the admin-only equivalent of AuthService.Register: it lets an
// admin assign an arbitrary role and bcrypt cost (the caller is expected to
// pass cfg.BCryptCost, never a hardcoded value) instead of always assigning
// "customer". It mirrors Register's password-strength enforcement and TOCTOU
// duplicate-key handling exactly, since a DTO length tag alone is not
// sufficient to enforce the letter+digit password policy (see Task 10).
func (s *UserService) CreateUser(req dto.CreateUserRequest, roles *repository.RoleRepo, cost int) (*model.User, error) {
	if !isStrongPassword(req.Password) {
		return nil, ErrWeakPassword
	}
	if _, err := s.users.FindByEmail(req.Email); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else {
		return nil, ErrEmailExists
	}
	if _, err := s.users.FindByUsername(req.Username); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else {
		return nil, ErrUsernameExists
	}
	role, err := roles.FindByName(req.Role)
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), cost)
	if err != nil {
		return nil, err
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	u := &model.User{
		ID:           uuid.New(),
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: string(hash),
		FullName:     req.FullName,
		Phone:        req.Phone,
		RoleID:       role.ID,
		IsActive:     isActive,
	}
	if err := s.users.Create(u); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// TOCTOU race: same guard as AuthService.Register — re-query to
			// determine which unique constraint was actually violated.
			if _, checkErr := s.users.FindByEmail(req.Email); checkErr == nil {
				return nil, ErrEmailExists
			}
			if _, checkErr := s.users.FindByUsername(req.Username); checkErr == nil {
				return nil, ErrUsernameExists
			}
			return nil, ErrConflict
		}
		return nil, err
	}
	if !isActive {
		// model.User.IsActive is tagged `gorm:"default:true"`, so for the Go
		// zero value (false) GORM's Create substitutes the tag's default into
		// both the INSERT and the in-memory struct field, silently turning an
		// explicit "inactive" request into an active user. Restore the
		// intended value on the struct, then persist it with Update (Save),
		// which does not have this substitution behavior.
		u.IsActive = false
		if err := s.users.Update(u); err != nil {
			return nil, err
		}
	}
	u.Role = *role
	return u, nil
}
