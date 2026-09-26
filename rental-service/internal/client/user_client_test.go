package client_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/rental-service/internal/client"
)

// newFakeServer is defined in product_client_test.go and shared by both test
// files in this package (client_test). Go does not allow declaring the same
// helper function twice within one package, so it is intentionally not
// re-declared here; see the self-review note in the task report.

func TestUserClient_VerifyCaller_ActiveTrue_ReturnsNil(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"active": true,
			},
		})
	})

	c := client.NewUserClient(url, "test-key")
	err := c.VerifyCaller("valid-token")

	require.NoError(t, err)
}

func TestUserClient_VerifyCaller_ActiveFalse_ReturnsErrAccountInactive(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"active": false,
			},
		})
	})

	c := client.NewUserClient(url, "test-key")
	err := c.VerifyCaller("valid-token")

	require.ErrorIs(t, err, client.ErrAccountInactive)
}

func TestUserClient_VerifyCaller_Unauthorized_ReturnsErrAccountInactive(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	c := client.NewUserClient(url, "test-key")
	err := c.VerifyCaller("bad-token")

	require.ErrorIs(t, err, client.ErrAccountInactive)
}

func TestUserClient_VerifyCaller_Forbidden_ReturnsErrAccountInactive(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	c := client.NewUserClient(url, "test-key")
	err := c.VerifyCaller("forbidden-token")

	require.ErrorIs(t, err, client.ErrAccountInactive)
}

func TestUserClient_VerifyCaller_ServerError_ReturnsErrDependency(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := client.NewUserClient(url, "test-key")
	err := c.VerifyCaller("some-token")

	require.ErrorIs(t, err, client.ErrDependency)
}

func TestUserClient_VerifyCaller_MalformedBody_ReturnsErrDependency(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	})

	c := client.NewUserClient(url, "test-key")
	err := c.VerifyCaller("some-token")

	require.ErrorIs(t, err, client.ErrDependency)
}

func TestUserClient_VerifyCaller_SendsInternalKeyHeaderAndTokenBody(t *testing.T) {
	var receivedKey, receivedMethod, receivedPath, receivedContentType string
	var receivedBody map[string]string
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-Internal-Key")
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedContentType = r.Header.Get("Content-Type")
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"active": true,
			},
		})
	})

	c := client.NewUserClient(url, "super-secret-key")
	err := c.VerifyCaller("the-caller-token")

	require.NoError(t, err)
	require.Equal(t, "super-secret-key", receivedKey)
	require.Equal(t, http.MethodPost, receivedMethod)
	require.Equal(t, "/auth/verify", receivedPath)
	require.Equal(t, "application/json", receivedContentType)
	require.Equal(t, map[string]string{"token": "the-caller-token"}, receivedBody)
}

func TestUserClient_VerifyCaller_UnreachableServer_ReturnsErrDependency(t *testing.T) {
	// Start and immediately close a server so its URL points at nothing
	// listening on that port -- the same failure mode a real unreachable
	// dependency (e.g. product-service down) produces.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := client.NewUserClient(url, "test-key")
	err := c.VerifyCaller("some-token")

	require.ErrorIs(t, err, client.ErrDependency)
}
