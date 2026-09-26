package repository_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/model"
	"github.com/equipment-rental-system/product-service/internal/repository"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Category{}, &model.Product{}))
	return db
}

func seedCategory(t *testing.T, db *gorm.DB, name string) model.Category {
	t.Helper()
	c := model.Category{ID: uuid.New(), Name: name}
	require.NoError(t, db.Create(&c).Error)
	return c
}

// newProduct builds a Product with an explicit ID — the model carries no
// GORM DB-side default for it (see model/product.go), so every caller that
// inserts one directly (bypassing ProductService.Create, which sets this
// itself) must supply one, matching user-service's convention.
func newProduct(categoryID uuid.UUID, name, status string) *model.Product {
	return &model.Product{ID: uuid.New(), CategoryID: categoryID, Name: name, Status: status}
}

func TestProductRepo_List_FiltersByQueryCategoryAndStatus(t *testing.T) {
	db := newTestDB(t)
	cameras := seedCategory(t, db, "กล้อง")
	audio := seedCategory(t, db, "เครื่องเสียง")

	repo := repository.NewProductRepo(db)
	require.NoError(t, repo.Create(newProduct(cameras.ID, "Canon EOS R5", model.StatusAvailable)))
	require.NoError(t, repo.Create(newProduct(cameras.ID, "Nikon Z9", model.StatusRented)))
	require.NoError(t, repo.Create(newProduct(audio.ID, "JBL Speaker", model.StatusAvailable)))

	t.Run("filters by category", func(t *testing.T) {
		products, total, err := repo.List(repository.ProductFilter{CategoryID: &cameras.ID, Page: 1, Limit: 20})
		require.NoError(t, err)
		require.EqualValues(t, 2, total)
		require.Len(t, products, 2)
	})

	t.Run("filters by status", func(t *testing.T) {
		products, total, err := repo.List(repository.ProductFilter{Status: model.StatusAvailable, Page: 1, Limit: 20})
		require.NoError(t, err)
		require.EqualValues(t, 2, total)
		require.Len(t, products, 2)
	})

	t.Run("searches by name case-insensitively", func(t *testing.T) {
		products, total, err := repo.List(repository.ProductFilter{Query: "canon", Page: 1, Limit: 20})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Equal(t, "Canon EOS R5", products[0].Name)
	})
}

func TestProductRepo_List_ClampsPageAndLimit(t *testing.T) {
	db := newTestDB(t)
	cat := seedCategory(t, db, "กล้อง")
	repo := repository.NewProductRepo(db)
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Create(newProduct(cat.ID, "item", model.StatusAvailable)))
	}

	// page=0 and limit=0 are both invalid; the repo must clamp rather than
	// divide-by-zero or return everything unpaginated.
	products, total, err := repo.List(repository.ProductFilter{Page: 0, Limit: 0})
	require.NoError(t, err)
	require.EqualValues(t, 5, total)
	require.Len(t, products, 5) // clamped limit defaults to 20, so all 5 fit on page 1
}

func TestProductRepo_List_RejectsUnknownSortColumn(t *testing.T) {
	db := newTestDB(t)
	cat := seedCategory(t, db, "กล้อง")
	repo := repository.NewProductRepo(db)
	require.NoError(t, repo.Create(newProduct(cat.ID, "item", model.StatusAvailable)))

	// An unrecognized sort column must fall back to created_at rather than
	// being interpolated into the ORDER BY clause unvalidated.
	_, _, err := repo.List(repository.ProductFilter{Sort: "status; DROP TABLE products;--", Page: 1, Limit: 20})
	require.NoError(t, err)
}

func TestProductRepo_UpdateStatus(t *testing.T) {
	db := newTestDB(t)
	cat := seedCategory(t, db, "กล้อง")
	repo := repository.NewProductRepo(db)
	p := newProduct(cat.ID, "item", model.StatusAvailable)
	require.NoError(t, repo.Create(p))

	updated, err := repo.UpdateStatus(p.ID, model.StatusRented)
	require.NoError(t, err)
	require.Equal(t, model.StatusRented, updated.Status)

	reloaded, err := repo.FindByID(p.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusRented, reloaded.Status)
}

func TestProductRepo_SoftDelete_ExcludesFromFindAndList(t *testing.T) {
	db := newTestDB(t)
	cat := seedCategory(t, db, "กล้อง")
	repo := repository.NewProductRepo(db)
	p := newProduct(cat.ID, "item", model.StatusAvailable)
	require.NoError(t, repo.Create(p))

	require.NoError(t, repo.SoftDelete(p.ID))

	_, err := repo.FindByID(p.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	_, total, err := repo.List(repository.ProductFilter{Page: 1, Limit: 20})
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
}
