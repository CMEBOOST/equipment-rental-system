package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/config"
)

// validSecret is exactly MinJWTSecretLen characters, so it is the shortest
// value Load() accepts.
var validSecret = strings.Repeat("a", config.MinJWTSecretLen)

// setValidEnv puts a complete, valid configuration in the environment. Each
// test then invalidates exactly one variable. t.Setenv restores the previous
// values when the test ends, so these tests don't leak into each other.
func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", validSecret)
	t.Setenv("INTERNAL_API_KEY", "dev-internal-key")
}

func TestLoad_ValidSecrets_Succeeds(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, validSecret, cfg.JWTSecret)
	require.Equal(t, "dev-internal-key", cfg.InternalAPIKey)
}

func TestLoad_EmptyJWTSecret_ReturnsError(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_SECRET", "")

	cfg, err := config.Load()

	require.ErrorIs(t, err, config.ErrMissingJWTSecret)
	require.Nil(t, cfg)
}

func TestLoad_ShortJWTSecret_ReturnsError(t *testing.T) {
	setValidEnv(t)
	// One character short of the design doc's §8.1 minimum.
	t.Setenv("JWT_SECRET", strings.Repeat("a", config.MinJWTSecretLen-1))

	cfg, err := config.Load()

	require.ErrorIs(t, err, config.ErrWeakJWTSecret)
	require.Nil(t, cfg)
}

func TestLoad_EmptyInternalAPIKey_ReturnsError(t *testing.T) {
	setValidEnv(t)
	t.Setenv("INTERNAL_API_KEY", "")

	cfg, err := config.Load()

	require.ErrorIs(t, err, config.ErrMissingInternalAPIKey)
	require.Nil(t, cfg)
}

// The repo's .env.example is the documented way to get a working local/dev
// setup ("cp .env.example .env"), so the secrets it ships must satisfy the
// validation above -- otherwise the documented recipe would no longer boot.
func TestLoad_EnvExampleSecrets_PassValidation(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_SECRET", "dev-secret-please-change-min-32-characters")
	t.Setenv("INTERNAL_API_KEY", "dev-internal-key")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(cfg.JWTSecret), config.MinJWTSecretLen)
}
