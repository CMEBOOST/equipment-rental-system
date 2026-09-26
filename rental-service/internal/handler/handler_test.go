package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/equipment-rental-system/rental-service/internal/client"
	"github.com/equipment-rental-system/rental-service/internal/dto"
	"github.com/equipment-rental-system/rental-service/internal/handler"
	"github.com/equipment-rental-system/rental-service/internal/model"
	"github.com/equipment-rental-system/rental-service/internal/repository"
	"github.com/equipment-rental-system/rental-service/internal/service"
)

// ---- newTestContext ------------------------------------------------------

func newTestContext(t *testing.T, method, path string, body any, userID uuid.UUID, role string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader io.Reader = bytes.NewReader([]byte(`{}`))
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	if userID != uuid.Nil {
		c.Set("user_id", userID)
		c.Set("role", role)
		c.Set("access_token", "fake-access-token")
	}
	return c, w
}

// ---- setupHandler ---------------------------------------------------------

// setupHandler wires a real *handler.RentalHandler against a real
// *service.RentalService, backed by a SQLite in-memory repository and
// httptest-backed product/user clients -- same driver/DDL as Tasks 6/9.
func setupHandler(t *testing.T, productHandler, userHandler http.HandlerFunc) (*handler.RentalHandler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE rentals (
			id          TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL,
			product_id  TEXT NOT NULL,
			start_date  DATETIME NOT NULL,
			due_date    DATETIME NOT NULL,
			return_date DATETIME,
			total_price NUMERIC NOT NULL,
			status      TEXT NOT NULL,
			created_at  DATETIME,
			updated_at  DATETIME
		)
	`).Error)

	repo := repository.NewRentalRepository(db)
	productSrv := httptest.NewServer(productHandler)
	t.Cleanup(productSrv.Close)
	userSrv := httptest.NewServer(userHandler)
	t.Cleanup(userSrv.Close)

	products := client.NewProductClient(productSrv.URL, "test-key")
	users := client.NewUserClient(userSrv.URL, "test-key")
	svc := service.NewRentalService(repo, products, users)
	return handler.NewRentalHandler(svc), db
}

// ---- canned upstream handlers ----------------------------------------------

func okVerifyCaller(active bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"active": active},
		})
	}
}

// failingVerify responds to POST /auth/verify with a 500, which UserClient
// turns into client.ErrDependency (any status >= 400 that isn't 401/403).
func failingVerify() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func availableProduct(pricePerDay float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":            uuid.New().String(),
				"price_per_day": pricePerDay,
				"status":        "available",
			},
		})
	}
}

func unavailableProduct() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":            uuid.New().String(),
				"price_per_day": 10.0,
				"status":        "rented",
			},
		})
	}
}

// notFoundProduct responds 404 to GET /products/{id}, driving client.ErrProductNotFound.
func notFoundProduct() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}
}

func statusOK() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

// combineProduct dispatches GET /products/{id} to get and
// PATCH /products/{id}/status to status, mirroring the two routes
// ProductClient actually calls.
func combineProduct(get, status http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			status(w, r)
			return
		}
		get(w, r)
	}
}

// ---- compensation-forcing helper -------------------------------------------

// forceUpdateFailure makes every subsequent UPDATE on this DB fail with a
// plain (unwrapped) error -- used to drive mapError's default branch through
// a real, unrecognized error surfacing from the repository.
func forceUpdateFailure(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Callback().Update().Before("gorm:update").
		Register("force_update_failure_handler_test", func(tx *gorm.DB) {
			tx.AddError(errors.New("forced update failure for test"))
		}))
}

// ---- misc test helpers ------------------------------------------------------

// seedRental writes a rental directly through the repository (bypassing the
// service) so Get/List/Approve/Return tests can start from a known state.
func seedRental(t *testing.T, db *gorm.DB, status string, userID uuid.UUID, startDate, dueDate time.Time) *model.Rental {
	t.Helper()
	repo := repository.NewRentalRepository(db)
	rental := &model.Rental{
		ID:         uuid.New(),
		UserID:     userID,
		ProductID:  uuid.New(),
		StartDate:  startDate,
		DueDate:    dueDate,
		TotalPrice: 100,
		Status:     status,
	}
	require.NoError(t, repo.Create(rental))
	return rental
}

// decodeBody unmarshals a recorded JSON response body into a generic map.
func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// errorCode extracts the "error.code" field from a decoded error response.
func errorCode(t *testing.T, resp map[string]any) string {
	t.Helper()
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok, "expected an error object in response: %#v", resp)
	code, _ := errObj["code"].(string)
	return code
}

// =============================================================================
// Functional coverage
// =============================================================================

func TestHandler_Create_InvalidBody_MissingUserID_ReturnsValidationError(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	body := map[string]any{
		"product_id": uuid.New().String(),
		"start_date": "2026-01-01",
		"due_date":   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals", body, uuid.New(), "admin")

	h.Create(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "VALIDATION_ERROR", errorCode(t, decodeBody(t, w)))
}

func TestHandler_Create_Success_Returns201(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	body := dto.CreateRentalRequest{
		UserID:    uuid.New().String(),
		ProductID: uuid.New().String(),
		StartDate: "2026-01-01",
		DueDate:   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals", body, uuid.New(), "admin")

	h.Create(c)

	require.Equal(t, http.StatusCreated, w.Code)
	resp := decodeBody(t, w)
	require.Equal(t, true, resp["success"])
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, model.StatusActive, data["status"])
}

func TestHandler_Request_Success_Returns201WithPendingStatus(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	userID := uuid.New()
	body := dto.RequestRentalRequest{
		ProductID: uuid.New().String(),
		StartDate: "2026-01-01",
		DueDate:   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals/request", body, userID, "customer")

	h.Request(c)

	require.Equal(t, http.StatusCreated, w.Code)
	resp := decodeBody(t, w)
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, model.StatusPending, data["status"])
	require.Equal(t, userID.String(), data["user_id"])
}

func TestHandler_Request_MissingUserContext_Returns401(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	body := dto.RequestRentalRequest{
		ProductID: uuid.New().String(),
		StartDate: "2026-01-01",
		DueDate:   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals/request", body, uuid.Nil, "")

	h.Request(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, "UNAUTHENTICATED", errorCode(t, decodeBody(t, w)))
}

func TestHandler_Get_InvalidUUIDParam_Returns404(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	c, w := newTestContext(t, http.MethodGet, "/rentals/not-a-uuid", nil, uuid.New(), "admin")
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}

	h.Get(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, "NOT_FOUND", errorCode(t, decodeBody(t, w)))
}

func TestHandler_Get_NotOwnerNotStaffNotAdmin_Returns403(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	owner := uuid.New()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rental := seedRental(t, db, model.StatusActive, owner, start, start.AddDate(0, 0, 10))

	requester := uuid.New() // different from owner
	c, w := newTestContext(t, http.MethodGet, "/rentals/"+rental.ID.String(), nil, requester, "customer")
	c.Params = gin.Params{{Key: "id", Value: rental.ID.String()}}

	h.Get(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, "FORBIDDEN", errorCode(t, decodeBody(t, w)))
}

func TestHandler_Get_Owner_Returns200(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	owner := uuid.New()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rental := seedRental(t, db, model.StatusActive, owner, start, start.AddDate(0, 0, 10))

	c, w := newTestContext(t, http.MethodGet, "/rentals/"+rental.ID.String(), nil, owner, "customer")
	c.Params = gin.Params{{Key: "id", Value: rental.ID.String()}}

	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	resp := decodeBody(t, w)
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, rental.ID.String(), data["id"])
}

func TestHandler_Get_Admin_CanViewAnyRental_Returns200(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	owner := uuid.New()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rental := seedRental(t, db, model.StatusActive, owner, start, start.AddDate(0, 0, 10))

	admin := uuid.New() // different from owner
	c, w := newTestContext(t, http.MethodGet, "/rentals/"+rental.ID.String(), nil, admin, "admin")
	c.Params = gin.Params{{Key: "id", Value: rental.ID.String()}}

	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_Approve_InvalidUUIDParam_Returns404(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	c, w := newTestContext(t, http.MethodPost, "/rentals/not-a-uuid/approve", nil, uuid.New(), "admin")
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}

	h.Approve(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, "NOT_FOUND", errorCode(t, decodeBody(t, w)))
}

func TestHandler_Return_InvalidBody_MissingReturnDate_ReturnsValidationError(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rental := seedRental(t, db, model.StatusActive, uuid.New(), start, start.AddDate(0, 0, 10))

	c, w := newTestContext(t, http.MethodPost, "/rentals/"+rental.ID.String()+"/return", nil, uuid.New(), "admin")
	c.Params = gin.Params{{Key: "id", Value: rental.ID.String()}}

	h.Return(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "VALIDATION_ERROR", errorCode(t, decodeBody(t, w)))
}

func TestHandler_List_ReturnsPaginationMeta(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedRental(t, db, model.StatusActive, uuid.New(), start, start.AddDate(0, 0, 10))
	seedRental(t, db, model.StatusActive, uuid.New(), start, start.AddDate(0, 0, 10))
	seedRental(t, db, model.StatusActive, uuid.New(), start, start.AddDate(0, 0, 10))

	c, w := newTestContext(t, http.MethodGet, "/rentals?page=1&limit=2", nil, uuid.New(), "admin")

	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	resp := decodeBody(t, w)
	meta, ok := resp["meta"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1), meta["page"])
	require.Equal(t, float64(2), meta["limit"])
	require.Equal(t, float64(3), meta["total"])
	require.Equal(t, float64(2), meta["total_pages"])
	data, ok := resp["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 2)
}

func TestHandler_MyList_FiltersToAuthenticatedUser(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	userA := uuid.New()
	userB := uuid.New()
	seedRental(t, db, model.StatusActive, userA, start, start.AddDate(0, 0, 10))
	seedRental(t, db, model.StatusActive, userA, start, start.AddDate(0, 0, 10))
	seedRental(t, db, model.StatusActive, userB, start, start.AddDate(0, 0, 10))

	c, w := newTestContext(t, http.MethodGet, "/rentals/mine", nil, userA, "customer")

	h.MyList(c)

	require.Equal(t, http.StatusOK, w.Code)
	resp := decodeBody(t, w)
	data, ok := resp["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 2)
	for _, item := range data {
		rentalMap, ok := item.(map[string]any)
		require.True(t, ok)
		require.Equal(t, userA.String(), rentalMap["user_id"])
	}
}

func TestHandler_Health_DBUp_Returns200(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	c, w := newTestContext(t, http.MethodGet, "/health", nil, uuid.Nil, "")

	handler.Health(db)(c)

	require.Equal(t, http.StatusOK, w.Code)
	resp := decodeBody(t, w)
	require.Equal(t, true, resp["success"])
}

func TestHandler_Health_DBClosed_Returns503(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	c, w := newTestContext(t, http.MethodGet, "/health", nil, uuid.Nil, "")

	handler.Health(db)(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// =============================================================================
// mapError branch coverage -- each driven through a real handler call whose
// service layer naturally returns that error.
// =============================================================================

func TestHandler_MapError_GormRecordNotFound_Returns404WithNotFoundCode(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	id := uuid.New() // not seeded -> repo.Get returns gorm.ErrRecordNotFound
	c, w := newTestContext(t, http.MethodGet, "/rentals/"+id.String(), nil, uuid.New(), "admin")
	c.Params = gin.Params{{Key: "id", Value: id.String()}}

	h.Get(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, "NOT_FOUND", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_ProductNotFound_Returns404WithNotFoundCode(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(notFoundProduct(), statusOK()), okVerifyCaller(true))

	body := dto.CreateRentalRequest{
		UserID:    uuid.New().String(),
		ProductID: uuid.New().String(),
		StartDate: "2026-01-01",
		DueDate:   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals", body, uuid.New(), "admin")

	h.Create(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, "NOT_FOUND", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_ProductUnavailable_Returns409WithConflictCode(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(unavailableProduct(), statusOK()), okVerifyCaller(true))

	body := dto.CreateRentalRequest{
		UserID:    uuid.New().String(),
		ProductID: uuid.New().String(),
		StartDate: "2026-01-01",
		DueDate:   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals", body, uuid.New(), "admin")

	h.Create(c)

	require.Equal(t, http.StatusConflict, w.Code)
	require.Equal(t, "CONFLICT", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_InvalidState_Returns409WithConflictCode(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	active := seedRental(t, db, model.StatusActive, uuid.New(), start, start.AddDate(0, 0, 10)) // not pending

	c, w := newTestContext(t, http.MethodPost, "/rentals/"+active.ID.String()+"/approve", nil, uuid.New(), "staff")
	c.Params = gin.Params{{Key: "id", Value: active.ID.String()}}

	h.Approve(c)

	require.Equal(t, http.StatusConflict, w.Code)
	require.Equal(t, "CONFLICT", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_InvalidDates_Returns400WithValidationErrorCode(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	body := dto.CreateRentalRequest{
		UserID:    uuid.New().String(),
		ProductID: uuid.New().String(),
		StartDate: "2026-01-10",
		DueDate:   "2026-01-01", // before start
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals", body, uuid.New(), "admin")

	h.Create(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "VALIDATION_ERROR", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_InvalidDateFormat_Returns400WithValidationErrorCode(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))

	body := dto.CreateRentalRequest{
		UserID:    uuid.New().String(),
		ProductID: uuid.New().String(),
		StartDate: "not-a-date",
		DueDate:   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals", body, uuid.New(), "admin")

	h.Create(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "VALIDATION_ERROR", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_InvalidReturnDate_Returns400WithValidationErrorCode(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	start := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	active := seedRental(t, db, model.StatusActive, uuid.New(), start, start.AddDate(0, 0, 10))

	body := dto.ReturnRentalRequest{ReturnDate: "2026-01-01"} // before start_date
	c, w := newTestContext(t, http.MethodPost, "/rentals/"+active.ID.String()+"/return", body, uuid.New(), "staff")
	c.Params = gin.Params{{Key: "id", Value: active.ID.String()}}

	h.Return(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "VALIDATION_ERROR", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_AccountInactive_Returns403WithAccountDisabledCode(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(false))

	body := dto.CreateRentalRequest{
		UserID:    uuid.New().String(),
		ProductID: uuid.New().String(),
		StartDate: "2026-01-01",
		DueDate:   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals", body, uuid.New(), "admin")

	h.Create(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, "ACCOUNT_DISABLED", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_Dependency_Returns503WithInternalErrorCode(t *testing.T) {
	h, _ := setupHandler(t, combineProduct(availableProduct(50), statusOK()), failingVerify())

	body := dto.CreateRentalRequest{
		UserID:    uuid.New().String(),
		ProductID: uuid.New().String(),
		StartDate: "2026-01-01",
		DueDate:   "2026-01-05",
	}
	c, w := newTestContext(t, http.MethodPost, "/rentals", body, uuid.New(), "admin")

	h.Create(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Equal(t, "INTERNAL_ERROR", errorCode(t, decodeBody(t, w)))
}

func TestHandler_MapError_UnknownError_Returns500WithInternalErrorCode(t *testing.T) {
	h, db := setupHandler(t, combineProduct(availableProduct(50), statusOK()), okVerifyCaller(true))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	active := seedRental(t, db, model.StatusActive, uuid.New(), start, start.AddDate(0, 0, 10))
	forceUpdateFailure(t, db)

	body := dto.ReturnRentalRequest{ReturnDate: "2026-01-05"}
	c, w := newTestContext(t, http.MethodPost, "/rentals/"+active.ID.String()+"/return", body, uuid.New(), "staff")
	c.Params = gin.Params{{Key: "id", Value: active.ID.String()}}

	h.Return(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Equal(t, "INTERNAL_ERROR", errorCode(t, decodeBody(t, w)))
}
