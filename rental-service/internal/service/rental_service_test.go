package service_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/equipment-rental-system/rental-service/internal/client"
	"github.com/equipment-rental-system/rental-service/internal/model"
	"github.com/equipment-rental-system/rental-service/internal/repository"
	"github.com/equipment-rental-system/rental-service/internal/service"
)

// ---- setupService -----------------------------------------------------

func setupService(t *testing.T, productHandler, userHandler http.HandlerFunc) (*service.RentalService, *gorm.DB) {
	t.Helper()
	// Silence gorm's default logger so an expected "record not found" (tested
	// deliberately below) doesn't print to test output.
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
	return service.NewRentalService(repo, products, users), db
}

// ---- canned upstream handlers ------------------------------------------

// okVerifyCaller responds to POST /auth/verify as if the caller's account has
// (or doesn't have) an active status.
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

// availableProduct responds to GET /products/{id} with an available product
// priced at pricePerDay.
func availableProduct(pricePerDay float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":            strings.TrimPrefix(r.URL.Path, "/products/"),
				"price_per_day": pricePerDay,
				"status":        "available",
			},
		})
	}
}

// unavailableProduct responds to GET /products/{id} with a product that is
// not available (already rented).
func unavailableProduct() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":            strings.TrimPrefix(r.URL.Path, "/products/"),
				"price_per_day": 10.0,
				"status":        "rented",
			},
		})
	}
}

// statusOK responds 200 OK to PATCH /products/{id}/status without recording
// anything -- used when a test doesn't care about the call order/count.
func statusOK() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

// trackingStatusHandler responds 200 OK to PATCH /products/{id}/status and
// records every status value it receives (mutex-protected) so a test can
// assert both the call count and the exact order of transitions.
func trackingStatusHandler(t *testing.T) (http.HandlerFunc, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var statuses []string

	handler := func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Status string `json:"status"`
		}
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&body)

		mu.Lock()
		statuses = append(statuses, body.Status)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(statuses))
		copy(out, statuses)
		return out
	}
	return handler, snapshot
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

// countingHandler wraps inner and counts every request it receives,
// regardless of method -- used to prove a short-circuit made zero calls.
func countingHandler(t *testing.T, inner http.HandlerFunc) (http.HandlerFunc, func() int) {
	t.Helper()
	var mu sync.Mutex
	count := 0

	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		inner(w, r)
	}
	getCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
	return handler, getCount
}

// ---- compensation-forcing helpers ---------------------------------------

// forceCreateFailure makes every subsequent INSERT into "rentals" on this DB
// fail -- used to exercise Create's compensating SetStatus("available")
// rollback. Registered on this test's own *gorm.DB instance only, so it
// never leaks between tests and never touches production code.
func forceCreateFailure(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Callback().Create().Before("gorm:create").
		Register("force_create_failure", func(tx *gorm.DB) {
			tx.AddError(errors.New("forced create failure for test"))
		}))
}

// forceUpdateFailure is the same trick for Approve/Return's repo.Update.
func forceUpdateFailure(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Callback().Update().Before("gorm:update").
		Register("force_update_failure", func(tx *gorm.DB) {
			tx.AddError(errors.New("forced update failure for test"))
		}))
}

// ---- misc test helpers ---------------------------------------------------

// seedRental writes a rental directly through the repository (bypassing the
// service) so Approve/Return/Get/List tests can start from a known state.
func seedRental(t *testing.T, db *gorm.DB, status string, startDate, dueDate time.Time) *model.Rental {
	t.Helper()
	repo := repository.NewRentalRepository(db)
	rental := &model.Rental{
		ID:         uuid.New(),
		UserID:     uuid.New(),
		ProductID:  uuid.New(),
		StartDate:  startDate,
		DueDate:    dueDate,
		TotalPrice: 100,
		Status:     status,
	}
	require.NoError(t, repo.Create(rental))
	return rental
}

// =========================================================================
// Create
// =========================================================================

func TestRentalService_Create_PendingRequest_DoesNotReserveProduct(t *testing.T) {
	statusHandler, statusCalls := trackingStatusHandler(t)
	svc, _ := setupService(t,
		combineProduct(availableProduct(50), statusHandler),
		okVerifyCaller(true),
	)

	userID, productID := uuid.New(), uuid.New()
	rental, err := svc.Create(userID, productID, "2026-01-01", "2026-01-05", "token", true)

	require.NoError(t, err)
	require.NotNil(t, rental)
	require.Equal(t, model.StatusPending, rental.Status)
	require.Empty(t, statusCalls(), "pending request must not reserve the product")
}

func TestRentalService_Create_DirectCreate_ReservesProductAndSetsStatusActive(t *testing.T) {
	statusHandler, statusCalls := trackingStatusHandler(t)
	svc, _ := setupService(t,
		combineProduct(availableProduct(50), statusHandler),
		okVerifyCaller(true),
	)

	userID, productID := uuid.New(), uuid.New()
	rental, err := svc.Create(userID, productID, "2026-01-01", "2026-01-05", "token", false)

	require.NoError(t, err)
	require.NotNil(t, rental)
	require.Equal(t, model.StatusActive, rental.Status)
	require.Equal(t, []string{"rented"}, statusCalls())
	require.Equal(t, 200.0, rental.TotalPrice) // 4 days * 50/day
}

func TestRentalService_Create_ProductUnavailable_ReturnsErrProductUnavailable(t *testing.T) {
	svc, _ := setupService(t,
		combineProduct(unavailableProduct(), statusOK()),
		okVerifyCaller(true),
	)

	rental, err := svc.Create(uuid.New(), uuid.New(), "2026-01-01", "2026-01-05", "token", false)

	require.ErrorIs(t, err, client.ErrProductUnavailable)
	require.Nil(t, rental)
}

func TestRentalService_Create_InvalidDates_DueBeforeStart_ReturnsErrInvalidDates(t *testing.T) {
	svc, _ := setupService(t,
		combineProduct(availableProduct(50), statusOK()),
		okVerifyCaller(true),
	)

	rental, err := svc.Create(uuid.New(), uuid.New(), "2026-01-10", "2026-01-01", "token", false)

	require.ErrorIs(t, err, service.ErrInvalidDates)
	require.Nil(t, rental)
}

func TestRentalService_Create_InvalidDateFormat_ReturnsErrInvalidDateFormat(t *testing.T) {
	svc, _ := setupService(t,
		combineProduct(availableProduct(50), statusOK()),
		okVerifyCaller(true),
	)

	rental, err := svc.Create(uuid.New(), uuid.New(), "not-a-date", "2026-01-05", "token", false)

	require.ErrorIs(t, err, service.ErrInvalidDateFormat)
	require.Nil(t, rental)
}

func TestRentalService_Create_VerifyCallerFails_ReturnsErrAccountInactive(t *testing.T) {
	productInner, productCalls := countingHandler(t, combineProduct(availableProduct(50), statusOK()))
	svc, _ := setupService(t, productInner, okVerifyCaller(false))

	rental, err := svc.Create(uuid.New(), uuid.New(), "2026-01-01", "2026-01-05", "bad-token", false)

	require.ErrorIs(t, err, client.ErrAccountInactive)
	require.Nil(t, rental)
	require.Equal(t, 0, productCalls(), "verification must short-circuit before any product call")
}

func TestRentalService_Create_RepoCreateFails_CompensatesBySettingProductAvailable(t *testing.T) {
	statusHandler, statusCalls := trackingStatusHandler(t)
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusHandler),
		okVerifyCaller(true),
	)
	forceCreateFailure(t, db)

	rental, err := svc.Create(uuid.New(), uuid.New(), "2026-01-01", "2026-01-05", "token", false)

	require.Error(t, err)
	require.Nil(t, rental)
	require.Equal(t, []string{"rented", "available"}, statusCalls())
}

// =========================================================================
// Approve
// =========================================================================

func TestRentalService_Approve_HappyPath_TransitionsPendingToActive(t *testing.T) {
	statusHandler, statusCalls := trackingStatusHandler(t)
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusHandler),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pending := seedRental(t, db, model.StatusPending, start, start.AddDate(0, 0, 10))

	rental, err := svc.Approve(pending.ID, "token")

	require.NoError(t, err)
	require.NotNil(t, rental)
	require.Equal(t, model.StatusActive, rental.Status)
	require.Equal(t, []string{"rented"}, statusCalls())

	persisted, err := svc.Get(pending.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusActive, persisted.Status)
}

func TestRentalService_Approve_WrongInitialState_ReturnsErrInvalidState(t *testing.T) {
	productInner, productCalls := countingHandler(t, combineProduct(availableProduct(50), statusOK()))
	svc, db := setupService(t, productInner, okVerifyCaller(true))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	active := seedRental(t, db, model.StatusActive, start, start.AddDate(0, 0, 10))

	rental, err := svc.Approve(active.ID, "token")

	require.ErrorIs(t, err, service.ErrInvalidState)
	require.Nil(t, rental)
	require.Equal(t, 0, productCalls(), "wrong-state check must short-circuit before any product call")
}

func TestRentalService_Approve_ProductUnavailable_ReturnsErrProductUnavailable(t *testing.T) {
	statusHandler, statusCalls := trackingStatusHandler(t)
	svc, db := setupService(t,
		combineProduct(unavailableProduct(), statusHandler),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pending := seedRental(t, db, model.StatusPending, start, start.AddDate(0, 0, 10))

	rental, err := svc.Approve(pending.ID, "token")

	require.ErrorIs(t, err, client.ErrProductUnavailable)
	require.Nil(t, rental)
	require.Empty(t, statusCalls(), "unavailable product must short-circuit before SetStatus")
}

func TestRentalService_Approve_RepoUpdateFails_CompensatesBySettingProductAvailable(t *testing.T) {
	statusHandler, statusCalls := trackingStatusHandler(t)
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusHandler),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pending := seedRental(t, db, model.StatusPending, start, start.AddDate(0, 0, 10))
	forceUpdateFailure(t, db)

	rental, err := svc.Approve(pending.ID, "token")

	require.Error(t, err)
	require.Nil(t, rental)
	require.Equal(t, []string{"rented", "available"}, statusCalls())
}

// =========================================================================
// Return
// =========================================================================

func TestRentalService_Return_HappyPath_TransitionsActiveToReturned(t *testing.T) {
	statusHandler, statusCalls := trackingStatusHandler(t)
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusHandler),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	active := seedRental(t, db, model.StatusActive, start, start.AddDate(0, 0, 10))

	rental, err := svc.Return(active.ID, "2026-01-05", "token")

	require.NoError(t, err)
	require.NotNil(t, rental)
	require.Equal(t, model.StatusReturned, rental.Status)
	require.NotNil(t, rental.ReturnDate)
	require.True(t, rental.ReturnDate.Equal(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, []string{"available"}, statusCalls())
}

func TestRentalService_Return_WrongInitialState_ReturnsErrInvalidState(t *testing.T) {
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusOK()),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pending := seedRental(t, db, model.StatusPending, start, start.AddDate(0, 0, 10))

	rental, err := svc.Return(pending.ID, "2026-01-05", "token")

	require.ErrorIs(t, err, service.ErrInvalidState)
	require.Nil(t, rental)
}

func TestRentalService_Return_InvalidReturnDateFormat_ReturnsErrInvalidDateFormat(t *testing.T) {
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusOK()),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	active := seedRental(t, db, model.StatusActive, start, start.AddDate(0, 0, 10))

	rental, err := svc.Return(active.ID, "not-a-date", "token")

	require.ErrorIs(t, err, service.ErrInvalidDateFormat)
	require.Nil(t, rental)
}

func TestRentalService_Return_ReturnDateBeforeStartDate_ReturnsErrInvalidReturnDate(t *testing.T) {
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusOK()),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	active := seedRental(t, db, model.StatusActive, start, start.AddDate(0, 0, 10))

	rental, err := svc.Return(active.ID, "2026-01-01", "token")

	require.ErrorIs(t, err, service.ErrInvalidReturnDate)
	require.Nil(t, rental)
}

func TestRentalService_Return_RepoUpdateFails_CompensatesBySettingProductRented(t *testing.T) {
	statusHandler, statusCalls := trackingStatusHandler(t)
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusHandler),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	active := seedRental(t, db, model.StatusActive, start, start.AddDate(0, 0, 10))
	forceUpdateFailure(t, db)

	rental, err := svc.Return(active.ID, "2026-01-05", "token")

	require.Error(t, err)
	require.Nil(t, rental)
	require.Equal(t, []string{"available", "rented"}, statusCalls())
}

// =========================================================================
// Get / List / IsNotFound
// =========================================================================

func TestRentalService_Get_DelegatesToRepository(t *testing.T) {
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusOK()),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seeded := seedRental(t, db, model.StatusActive, start, start.AddDate(0, 0, 10))

	found, err := svc.Get(seeded.ID)

	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, seeded.ID, found.ID)
	require.Equal(t, seeded.UserID, found.UserID)
	require.Equal(t, seeded.Status, found.Status)
}

func TestRentalService_Get_NotFound_ReturnsGormErrRecordNotFound(t *testing.T) {
	svc, _ := setupService(t,
		combineProduct(availableProduct(50), statusOK()),
		okVerifyCaller(true),
	)

	found, err := svc.Get(uuid.New())

	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Nil(t, found)
}

func TestRentalService_List_DelegatesToRepositoryWithFilter(t *testing.T) {
	svc, db := setupService(t,
		combineProduct(availableProduct(50), statusOK()),
		okVerifyCaller(true),
	)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedRental(t, db, model.StatusPending, start, start.AddDate(0, 0, 10))
	seedRental(t, db, model.StatusActive, start, start.AddDate(0, 0, 10))
	seedRental(t, db, model.StatusActive, start, start.AddDate(0, 0, 10))

	rentals, total, err := svc.List(repository.RentalFilter{Status: model.StatusActive})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rentals, 2)
	for _, r := range rentals {
		require.Equal(t, model.StatusActive, r.Status)
	}
}

func TestRentalService_IsNotFound_RecognizesGormErrRecordNotFound(t *testing.T) {
	require.True(t, service.IsNotFound(gorm.ErrRecordNotFound))
}

func TestRentalService_IsNotFound_RecognizesErrProductNotFound(t *testing.T) {
	require.True(t, service.IsNotFound(client.ErrProductNotFound))
}

func TestRentalService_IsNotFound_FalseForUnrelatedError(t *testing.T) {
	require.False(t, service.IsNotFound(errors.New("something else entirely")))
}
