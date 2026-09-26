package router_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/config"
	"github.com/equipment-rental-system/product-service/internal/middleware"
	"github.com/equipment-rental-system/product-service/internal/model"
	"github.com/equipment-rental-system/product-service/internal/router"
)

const testInternalKey = "test-internal-key"

func newTestRouter(t *testing.T) (*gorm.DB, http.Handler) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Category{}, &model.Product{}))

	cfg := &config.Config{InternalAPIKey: testInternalKey, CORSOrigin: config.DefaultCORSOrigin}
	return db, router.New(db, cfg)
}

func bearerFor(t *testing.T, role string) string {
	t.Helper()
	claims := middleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "11111111-1111-1111-1111-111111111111",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Signed with a secret product-service never checks — Kong would verify
	// this in production; the router itself only decodes claims.
	s, err := token.SignedString([]byte("not-the-real-jwt-secret"))
	require.NoError(t, err)
	return "Bearer " + s
}

func doJSON(t *testing.T, h http.Handler, method, path, auth string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestRouter_ListProducts_RequiresAuthButAnyRole(t *testing.T) {
	_, h := newTestRouter(t)

	w := doJSON(t, h, http.MethodGet, "/api/v1/products", "", nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	w = doJSON(t, h, http.MethodGet, "/api/v1/products", bearerFor(t, "customer"), nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_CreateProduct_AdminOrStaffOnly(t *testing.T) {
	db, h := newTestRouter(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)
	body := map[string]any{"category_id": cat.ID.String(), "name": "Canon EOS R5", "price_per_day": 1200}

	w := doJSON(t, h, http.MethodPost, "/api/v1/products", bearerFor(t, "customer"), body)
	require.Equal(t, http.StatusForbidden, w.Code)

	w = doJSON(t, h, http.MethodPost, "/api/v1/products", bearerFor(t, "staff"), body)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data.ID)
}

func TestRouter_DeleteProduct_AdminOnly_StaffForbidden(t *testing.T) {
	db, h := newTestRouter(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)
	p := model.Product{ID: uuid.New(), CategoryID: cat.ID, Name: "x", Status: model.StatusAvailable}
	require.NoError(t, db.Create(&p).Error)

	w := doJSON(t, h, http.MethodDelete, "/api/v1/products/"+p.ID.String(), bearerFor(t, "staff"), nil)
	require.Equal(t, http.StatusForbidden, w.Code)

	w = doJSON(t, h, http.MethodDelete, "/api/v1/products/"+p.ID.String(), bearerFor(t, "admin"), nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ChangeStatus_ViaInternalKey_NoTokenNeeded(t *testing.T) {
	db, h := newTestRouter(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)
	p := model.Product{ID: uuid.New(), CategoryID: cat.ID, Name: "x", Status: model.StatusAvailable}
	require.NoError(t, db.Create(&p).Error)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/products/"+p.ID.String()+"/status",
		bytes.NewBufferString(`{"status":"rented"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", testInternalKey)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ChangeStatus_ToRented_Returns409WhenNotAvailable(t *testing.T) {
	db, h := newTestRouter(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)
	p := model.Product{ID: uuid.New(), CategoryID: cat.ID, Name: "x", Status: model.StatusRented}
	require.NoError(t, db.Create(&p).Error)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/products/"+p.ID.String()+"/status",
		bytes.NewBufferString(`{"status":"rented"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", testInternalKey)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.Equal(t, "CONFLICT", resp.Error.Code)
}

func TestRouter_GetProduct_NotFound(t *testing.T) {
	_, h := newTestRouter(t)
	w := doJSON(t, h, http.MethodGet, "/api/v1/products/11111111-1111-1111-1111-111111111111",
		bearerFor(t, "customer"), nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestRouter_Health_Public(t *testing.T) {
	_, h := newTestRouter(t)
	w := doJSON(t, h, http.MethodGet, "/health", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ListCategories_RequiresAuth(t *testing.T) {
	_, h := newTestRouter(t)
	w := doJSON(t, h, http.MethodGet, "/api/v1/categories", "", nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	w = doJSON(t, h, http.MethodGet, "/api/v1/categories", bearerFor(t, "customer"), nil)
	require.Equal(t, http.StatusOK, w.Code)
}
