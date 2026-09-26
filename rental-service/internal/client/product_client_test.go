package client_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/rental-service/internal/client"
)

func newFakeServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestProductClient_Get_Success_ReturnsProduct(t *testing.T) {
	id := uuid.New()
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":            id.String(),
				"price_per_day": 50.5,
				"status":        "available",
			},
		})
	})

	c := client.NewProductClient(url, "test-key")
	product, err := c.Get(id)

	require.NoError(t, err)
	require.NotNil(t, product)
	require.Equal(t, id, product.ID)
	require.Equal(t, 50.5, product.PricePerDay)
	require.Equal(t, "available", product.Status)
}

func TestProductClient_Get_NotFound_ReturnsErrProductNotFound(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	c := client.NewProductClient(url, "test-key")
	product, err := c.Get(uuid.New())

	require.ErrorIs(t, err, client.ErrProductNotFound)
	require.Nil(t, product)
}

func TestProductClient_Get_ServerError_ReturnsErrDependency(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := client.NewProductClient(url, "test-key")
	product, err := c.Get(uuid.New())

	require.ErrorIs(t, err, client.ErrDependency)
	require.Nil(t, product)
}

func TestProductClient_Get_OtherClientError_ReturnsErrDependency(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	c := client.NewProductClient(url, "test-key")
	product, err := c.Get(uuid.New())

	require.ErrorIs(t, err, client.ErrDependency)
	require.Nil(t, product)
}

func TestProductClient_Get_MalformedBody_ReturnsErrDependency(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	})

	c := client.NewProductClient(url, "test-key")
	product, err := c.Get(uuid.New())

	require.ErrorIs(t, err, client.ErrDependency)
	require.Nil(t, product)
}

func TestProductClient_Get_SuccessFalseInBody_ReturnsErrDependency(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"data":    map[string]any{},
		})
	})

	c := client.NewProductClient(url, "test-key")
	product, err := c.Get(uuid.New())

	require.ErrorIs(t, err, client.ErrDependency)
	require.Nil(t, product)
}

func TestProductClient_Get_SendsInternalKeyHeader(t *testing.T) {
	id := uuid.New()
	var receivedKey string
	var receivedMethod, receivedPath string
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-Internal-Key")
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":            id.String(),
				"price_per_day": 10.0,
				"status":        "available",
			},
		})
	})

	c := client.NewProductClient(url, "super-secret-key")
	_, err := c.Get(id)

	require.NoError(t, err)
	require.Equal(t, "super-secret-key", receivedKey)
	require.Equal(t, http.MethodGet, receivedMethod)
	require.Equal(t, "/products/"+id.String(), receivedPath)
}

func TestProductClient_SetStatus_Success_ReturnsNil(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	c := client.NewProductClient(url, "test-key")
	err := c.SetStatus(uuid.New(), "rented")

	require.NoError(t, err)
}

func TestProductClient_SetStatus_NotFound_ReturnsErrProductNotFound(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	c := client.NewProductClient(url, "test-key")
	err := c.SetStatus(uuid.New(), "rented")

	require.ErrorIs(t, err, client.ErrProductNotFound)
}

func TestProductClient_SetStatus_Conflict_ReturnsErrProductUnavailable(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})

	c := client.NewProductClient(url, "test-key")
	err := c.SetStatus(uuid.New(), "rented")

	require.ErrorIs(t, err, client.ErrProductUnavailable)
}

func TestProductClient_SetStatus_OtherClientError_ReturnsErrDependency(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	c := client.NewProductClient(url, "test-key")
	err := c.SetStatus(uuid.New(), "rented")

	require.ErrorIs(t, err, client.ErrDependency)
}

func TestProductClient_SetStatus_ServerError_ReturnsErrDependency(t *testing.T) {
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := client.NewProductClient(url, "test-key")
	err := c.SetStatus(uuid.New(), "rented")

	require.ErrorIs(t, err, client.ErrDependency)
}

func TestProductClient_SetStatus_SendsCorrectBodyAndMethod(t *testing.T) {
	id := uuid.New()
	var receivedMethod, receivedPath, receivedContentType, receivedKey string
	var receivedBody map[string]string
	url := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedContentType = r.Header.Get("Content-Type")
		receivedKey = r.Header.Get("X-Internal-Key")
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &receivedBody)
		w.WriteHeader(http.StatusOK)
	})

	c := client.NewProductClient(url, "test-key")
	err := c.SetStatus(id, "rented")

	require.NoError(t, err)
	require.Equal(t, http.MethodPatch, receivedMethod)
	require.Equal(t, "/products/"+id.String()+"/status", receivedPath)
	require.Equal(t, "application/json", receivedContentType)
	require.Equal(t, "test-key", receivedKey)
	require.Equal(t, map[string]string{"status": "rented"}, receivedBody)
}
