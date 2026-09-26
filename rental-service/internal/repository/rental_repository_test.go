package repository_test

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/equipment-rental-system/rental-service/internal/model"
	"github.com/equipment-rental-system/rental-service/internal/repository"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Silence gorm's default logger so an expected "record not found" (tested
	// deliberately below) doesn't print to test output.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	return db
}

func newRental(userID, productID uuid.UUID, status string, createdAt time.Time) *model.Rental {
	start := createdAt.Add(24 * time.Hour)
	return &model.Rental{
		ID: uuid.New(), UserID: userID, ProductID: productID,
		StartDate: start, DueDate: start.Add(48 * time.Hour),
		TotalPrice: 100, Status: status, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func TestRentalRepository_Create_Succeeds(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	rental := newRental(uuid.New(), uuid.New(), model.StatusPending, time.Now())

	err := repo.Create(rental)
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&model.Rental{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestRentalRepository_Get_Found_ReturnsRental(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	rental := newRental(uuid.New(), uuid.New(), model.StatusActive, time.Now())
	require.NoError(t, repo.Create(rental))

	found, err := repo.Get(rental.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, rental.ID, found.ID)
	assert.Equal(t, rental.UserID, found.UserID)
	assert.Equal(t, rental.Status, found.Status)
}

func TestRentalRepository_Get_NotFound_ReturnsGormErrRecordNotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	found, err := repo.Get(uuid.New())

	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Nil(t, found)
}

func TestRentalRepository_Update_PersistsStatusChange(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	rental := newRental(uuid.New(), uuid.New(), model.StatusPending, time.Now())
	require.NoError(t, repo.Create(rental))

	rental.Status = model.StatusReturned
	require.NoError(t, repo.Update(rental))

	found, err := repo.Get(rental.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusReturned, found.Status)
}

func TestRentalRepository_List_FiltersByStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	now := time.Now()
	require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusPending, now)))
	require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusActive, now)))
	require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusActive, now)))

	rentals, total, err := repo.List(repository.RentalFilter{Status: model.StatusActive})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rentals, 2)
	for _, r := range rentals {
		assert.Equal(t, model.StatusActive, r.Status)
	}
}

func TestRentalRepository_List_FiltersByUserID(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	targetUser := uuid.New()
	now := time.Now()
	require.NoError(t, repo.Create(newRental(targetUser, uuid.New(), model.StatusPending, now)))
	require.NoError(t, repo.Create(newRental(targetUser, uuid.New(), model.StatusActive, now)))
	require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusActive, now)))

	rentals, total, err := repo.List(repository.RentalFilter{UserID: targetUser.String()})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rentals, 2)
	for _, r := range rentals {
		assert.Equal(t, targetUser, r.UserID)
	}
}

func TestRentalRepository_List_DefaultsSortToCreatedAtDesc(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	base := time.Now().Truncate(time.Second)
	oldest := newRental(uuid.New(), uuid.New(), model.StatusPending, base)
	middle := newRental(uuid.New(), uuid.New(), model.StatusPending, base.Add(time.Hour))
	newest := newRental(uuid.New(), uuid.New(), model.StatusPending, base.Add(2*time.Hour))
	require.NoError(t, repo.Create(oldest))
	require.NoError(t, repo.Create(middle))
	require.NoError(t, repo.Create(newest))

	rentals, _, err := repo.List(repository.RentalFilter{})
	require.NoError(t, err)
	require.Len(t, rentals, 3)
	require.Equal(t, newest.ID, rentals[0].ID)
	require.Equal(t, middle.ID, rentals[1].ID)
	require.Equal(t, oldest.ID, rentals[2].ID)
}

func TestRentalRepository_List_ValidSort_StartDateAscending(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	base := time.Now().Truncate(time.Second)
	first := newRental(uuid.New(), uuid.New(), model.StatusPending, base)
	second := newRental(uuid.New(), uuid.New(), model.StatusPending, base.Add(time.Hour))
	third := newRental(uuid.New(), uuid.New(), model.StatusPending, base.Add(2*time.Hour))
	require.NoError(t, repo.Create(third))
	require.NoError(t, repo.Create(first))
	require.NoError(t, repo.Create(second))

	rentals, _, err := repo.List(repository.RentalFilter{Sort: "start_date", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, rentals, 3)
	require.Equal(t, first.ID, rentals[0].ID)
	require.Equal(t, second.ID, rentals[1].ID)
	require.Equal(t, third.ID, rentals[2].ID)
}

func TestRentalRepository_List_UnlistedSortColumn_FallsBackToCreatedAt(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	base := time.Now().Truncate(time.Second)
	oldest := newRental(uuid.New(), uuid.New(), model.StatusPending, base)
	newest := newRental(uuid.New(), uuid.New(), model.StatusPending, base.Add(time.Hour))
	require.NoError(t, repo.Create(oldest))
	require.NoError(t, repo.Create(newest))

	rentals, total, err := repo.List(repository.RentalFilter{Sort: "'; DROP TABLE rentals; --"})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rentals, 2)
	// Falls back to created_at desc, and the table must still exist (no injection occurred).
	require.Equal(t, newest.ID, rentals[0].ID)
	require.Equal(t, oldest.ID, rentals[1].ID)

	var count int64
	require.NoError(t, db.Model(&model.Rental{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestRentalRepository_List_InvalidOrder_FallsBackToDesc(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	base := time.Now().Truncate(time.Second)
	oldest := newRental(uuid.New(), uuid.New(), model.StatusPending, base)
	newest := newRental(uuid.New(), uuid.New(), model.StatusPending, base.Add(time.Hour))
	require.NoError(t, repo.Create(oldest))
	require.NoError(t, repo.Create(newest))

	rentals, _, err := repo.List(repository.RentalFilter{Order: "sideways"})
	require.NoError(t, err)
	require.Len(t, rentals, 2)
	require.Equal(t, newest.ID, rentals[0].ID)
	require.Equal(t, oldest.ID, rentals[1].ID)
}

func TestRentalRepository_List_Pagination_DefaultsPageAndLimit(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	now := time.Now()
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusPending, now.Add(time.Duration(i)*time.Minute))))
	}

	// Page 0 / Limit 0 should default to page 1, limit 20 - all 3 rows returned.
	rentals, total, err := repo.List(repository.RentalFilter{Page: 0, Limit: 0})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, rentals, 3)

	// Limit above the max (100) should clamp sanely (repository falls back to 20).
	rentals, total, err = repo.List(repository.RentalFilter{Page: 1, Limit: 150})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, rentals, 3)
}

func TestRentalRepository_List_TotalCountReflectsFilterNotWholeTable(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewRentalRepository(db)

	now := time.Now()
	require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusPending, now)))
	require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusActive, now)))
	require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusActive, now)))
	require.NoError(t, repo.Create(newRental(uuid.New(), uuid.New(), model.StatusReturned, now)))

	rentals, total, err := repo.List(repository.RentalFilter{Status: model.StatusActive, Limit: 1})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rentals, 1)
}
