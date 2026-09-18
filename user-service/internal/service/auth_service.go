package service

import (
	"errors"
	"regexp"
	"time"

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
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
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
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// TOCTOU race: Create() failed on a unique constraint violation. Re-query
			// for email and username to determine which one was violated. This is
			// driver-agnostic because GORM v2 normalizes unique-constraint violations
			// to gorm.ErrDuplicatedKey across drivers (SQLite, PostgreSQL, etc.).
			if _, checkErr := s.users.FindByEmail(req.Email); checkErr == nil {
				return nil, ErrEmailExists
			}
			if _, checkErr := s.users.FindByUsername(req.Username); checkErr == nil {
				return nil, ErrUsernameExists
			}
			// Practically impossible case: constraint violated but re-queries found nothing.
			return nil, ErrConflict
		}
		// Any other Create() error (connection failure, ambiguous write, etc.) is
		// propagated directly rather than being misreported as a duplicate via re-query.
		return nil, err
	}
	u.Role = *role
	return u, nil
}

// UsersRepoForTest exposes the user repository for test setup only (e.g.
// disabling an account after registration). Not intended for production use.
func (s *AuthService) UsersRepoForTest() *repository.UserRepo { return s.users }

// Login authenticates a user by email/password, records an audit entry in
// login_logs for every attempt (success or failure), and on success issues a
// new JWT access token plus an opaque refresh token.
func (s *AuthService) Login(req dto.LoginRequest, ip, userAgent string) (string, string, int, *model.User, error) {
	var user *model.User
	success := false

	// This defer always runs, regardless of which return statement below is
	// hit, so every login attempt — success or failure, including unexpected
	// errors — is recorded in login_logs exactly once.
	defer func() {
		logEntry := &model.LoginLog{
			EmailAttempted: req.Email,
			Success:        success,
			IPAddress:      ip,
			UserAgent:      userAgent,
		}
		if user != nil {
			id := user.ID
			logEntry.UserID = &id
		}
		_ = s.logs.Create(logEntry)
	}()

	u, err := s.users.FindByEmail(req.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", 0, nil, ErrInvalidCredentials
		}
		// Unexpected error (DB connection issue, etc.) — propagate rather than
		// misreporting it as invalid credentials.
		return "", "", 0, nil, err
	}
	user = u

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		return "", "", 0, nil, ErrInvalidCredentials
	}
	if !user.IsActive {
		return "", "", 0, nil, ErrAccountDisabled
	}

	access, expiresIn, err := s.tokens.GenerateAccessToken(*user)
	if err != nil {
		return "", "", 0, nil, err
	}
	rawRefresh, err := NewOpaqueRefreshToken()
	if err != nil {
		return "", "", 0, nil, err
	}
	if err := s.refreshTokens.Create(&model.RefreshToken{
		UserID:    user.ID,
		TokenHash: HashRefreshToken(rawRefresh),
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshTTL),
		IPAddress: ip,
		UserAgent: userAgent,
	}); err != nil {
		return "", "", 0, nil, err
	}

	success = true
	return access, rawRefresh, expiresIn, user, nil
}

// Refresh rotates a valid opaque refresh token: the presented token is
// revoked and a brand-new access/refresh token pair is issued. Any problem
// with the presented token (unknown, revoked, expired, or an unexpected
// lookup error) is reported uniformly as ErrInvalidRefreshToken so callers
// can't distinguish those cases from token contents.
func (s *AuthService) Refresh(rawToken string) (string, string, int, error) {
	hash := HashRefreshToken(rawToken)
	rt, err := s.refreshTokens.FindByHash(hash)
	if err != nil || rt.RevokedAt != nil || rt.ExpiresAt.Before(time.Now()) {
		return "", "", 0, ErrInvalidRefreshToken
	}
	u, err := s.users.FindByID(rt.UserID)
	if err != nil {
		return "", "", 0, ErrInvalidRefreshToken
	}
	if !u.IsActive {
		return "", "", 0, ErrAccountDisabled
	}
	if err := s.refreshTokens.RevokeByHash(hash); err != nil {
		return "", "", 0, err
	}
	access, expiresIn, err := s.tokens.GenerateAccessToken(*u)
	if err != nil {
		return "", "", 0, err
	}
	newRaw, err := NewOpaqueRefreshToken()
	if err != nil {
		return "", "", 0, err
	}
	if err := s.refreshTokens.Create(&model.RefreshToken{
		UserID: u.ID, TokenHash: HashRefreshToken(newRaw), ExpiresAt: time.Now().Add(s.cfg.JWTRefreshTTL),
	}); err != nil {
		return "", "", 0, err
	}
	return access, newRaw, expiresIn, nil
}

// Logout revokes the presented refresh token so it can no longer be used to
// obtain new tokens.
func (s *AuthService) Logout(rawToken string) error {
	return s.refreshTokens.RevokeByHash(HashRefreshToken(rawToken))
}
