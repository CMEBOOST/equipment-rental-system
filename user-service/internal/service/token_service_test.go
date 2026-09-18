package service_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func TestTokenService_GenerateAndParse_RoundTrip(t *testing.T) {
	ts := service.NewTokenService("test-secret-min-32-characters-ok", 15*time.Minute)
	u := model.User{
		ID:       uuid.New(),
		Email:    "somchai@example.com",
		Username: "somchai",
		Role:     model.Role{Name: "customer"},
	}

	token, expiresIn, err := ts.GenerateAccessToken(u)
	require.NoError(t, err)
	require.Equal(t, 900, expiresIn)

	claims, err := ts.ParseAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, u.ID.String(), claims.Sub)
	require.Equal(t, "customer", claims.Role)
	require.Equal(t, "equipment-rental-system", claims.Iss)
}

func TestHashRefreshToken_Deterministic(t *testing.T) {
	raw, err := service.NewOpaqueRefreshToken()
	require.NoError(t, err)
	require.Len(t, raw, 64)
	require.Equal(t, service.HashRefreshToken(raw), service.HashRefreshToken(raw))
}
