package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/equipment-rental-system/user-service/internal/model"
)

const tokenIssuer = "equipment-rental-system"

type Claims struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Iss      string `json:"iss"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret    []byte
	accessTTL time.Duration
}

func NewTokenService(secret string, accessTTL time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), accessTTL: accessTTL}
}

func (s *TokenService) GenerateAccessToken(u model.User) (string, int, error) {
	now := time.Now()
	claims := Claims{
		Sub:      u.ID.String(),
		Email:    u.Email,
		Username: u.Username,
		Role:     u.Role.Name,
		Iss:      tokenIssuer,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
			ID:        uuid.New().String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", 0, err
	}
	return signed, int(s.accessTTL.Seconds()), nil
}

func (s *TokenService) ParseAccessToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func NewOpaqueRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// uuidParse parses a JWT "sub" claim (a user ID) into a uuid.UUID. Small
// wrapper kept here alongside the other token-related helpers so callers in
// this package don't need to import uuid directly just for this one call.
func uuidParse(s string) (uuid.UUID, error) { return uuid.Parse(s) }
