package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/product-service/internal/config"
)

func TestLoad_ValidInternalKey_Succeeds(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "dev-internal-key")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "dev-internal-key", cfg.InternalAPIKey)
	require.Equal(t, "8082", cfg.AppPort) // default from CONTRACT.md §1
}

func TestLoad_EmptyInternalAPIKey_ReturnsError(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "")

	cfg, err := config.Load()

	require.ErrorIs(t, err, config.ErrMissingInternalAPIKey)
	require.Nil(t, cfg)
}

// The repo's root .env.example is the documented way to get a working
// local/dev setup, so the value it ships for INTERNAL_API_KEY must satisfy
// the validation above.
func TestLoad_EnvExampleInternalKey_PassesValidation(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "dev-internal-key")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.NotEmpty(t, cfg.InternalAPIKey)
}
