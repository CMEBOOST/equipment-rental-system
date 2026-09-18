package service

import (
	"errors"
	"regexp"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

var (
	ErrEmailExists      = errors.New("email already exists")
	ErrUsernameExists   = errors.New("username already exists")
	ErrWeakPassword     = errors.New("password does not meet strength requirements")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled  = errors.New("account disabled")
	ErrConflict         = errors.New("conflict")
)

var passwordHasDigitAndLetter = regexp.MustCompile(`^.*[A-Za-z].*$`)
var passwordHasDigit = regexp.MustCompile(`^.*[0-9].*$`)

type AuthService struct {
	users         *repository.UserRepo
	roles         *repository.RoleRepo
	refreshTokens *repository.RefreshTokenRepo
	logs          *repository.LoginLogRepo
	tokens        *TokenService
	cfg           *config.Config
}

func NewAuthService(users *repository.UserRepo, roles *repository.RoleRepo,
	refreshTokens *repository.RefreshTokenRepo, logs *repository.LoginLogRepo,
	tokens *TokenService, cfg *config.Config) *AuthService {
	return &AuthService{users: users, roles: roles, refreshTokens: refreshTokens, logs: logs, tokens: tokens, cfg: cfg}
}

func isStrongPassword(pw string) bool {
	return len(pw) >= 8 && passwordHasDigitAndLetter.MatchString(pw) && passwordHasDigit.MatchString(pw)
}

func (s *AuthService) Register(req dto.RegisterRequest) (*model.User, error) {
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
	role, err := s.roles.FindByName("customer")
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.cfg.BCryptCost)
	if err != nil {
		return nil, err
	}
	u := &model.User{
		ID:           uuid.New(),
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: string(hash),
		FullName:     req.FullName,
		Phone:        req.Phone,
		RoleID:       role.ID,
		IsActive:     true,
	}
	if err := s.users.Create(u); err != nil {
		// TOCTOU race: when Create() fails, attempt to determine if it's due to unique constraint
		// by re-querying for email and username. This works across all drivers (SQLite, PostgreSQL, etc.)
		if _, checkErr := s.users.FindByEmail(req.Email); checkErr == nil {
			return nil, ErrEmailExists
		}
		if _, checkErr := s.users.FindByUsername(req.Username); checkErr == nil {
			return nil, ErrUsernameExists
		}
		// If re-queries found nothing, it's a different kind of error - propagate it
		return nil, err
	}
	u.Role = *role
	return u, nil
}
