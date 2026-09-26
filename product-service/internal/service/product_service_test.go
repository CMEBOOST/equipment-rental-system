package service_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/model"
	"github.com/equipment-rental-system/product-service/internal/repository"
	"github.com/equipment-rental-system/product-service/internal/service"
)

func newTestService(t *testing.T) (*service.ProductService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Category{}, &model.Product{}))
	categoryRepo := repository.NewCategoryRepo(db)
	productRepo := repository.NewProductRepo(db)
	return service.NewProductService(productRepo, categoryRepo), db
}

func TestProductService_Create_RejectsUnknownCategory(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Create(uuid.New(), "Canon EOS R5", "", 1200, "")
	require.ErrorIs(t, err, service.ErrCategoryNotFound)
}

func TestProductService_Create_Succeeds(t *testing.T) {
	svc, db := newTestService(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)

	p, err := svc.Create(cat.ID, "Canon EOS R5", "full frame", 1200, "http://x/img.jpg")
	require.NoError(t, err)
	require.Equal(t, "Canon EOS R5", p.Name)
	require.Equal(t, model.StatusAvailable, p.Status)
	require.Equal(t, "กล้อง", p.Category.Name) // reloaded with the category preloaded
}

func TestProductService_Update_PartialUpdateLeavesOtherFieldsUnchanged(t *testing.T) {
	svc, db := newTestService(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)
	p, err := svc.Create(cat.ID, "Canon EOS R5", "original description", 1200, "")
	require.NoError(t, err)

	newPrice := 1500.0
	updated, err := svc.Update(p.ID, nil, nil, nil, nil, &newPrice)
	require.NoError(t, err)
	require.Equal(t, "Canon EOS R5", updated.Name)                // untouched
	require.Equal(t, "original description", updated.Description) // untouched
	require.Equal(t, 1500.0, updated.PricePerDay)                 // changed
}

func TestProductService_Update_RejectsUnknownCategory(t *testing.T) {
	svc, db := newTestService(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)
	p, err := svc.Create(cat.ID, "Canon EOS R5", "", 1200, "")
	require.NoError(t, err)

	bogus := uuid.New()
	_, err = svc.Update(p.ID, &bogus, nil, nil, nil, nil)
	require.ErrorIs(t, err, service.ErrCategoryNotFound)
}

func TestProductService_Delete_RejectsRentedProduct(t *testing.T) {
	svc, db := newTestService(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)
	p, err := svc.Create(cat.ID, "Canon EOS R5", "", 1200, "")
	require.NoError(t, err)
	_, err = svc.ChangeStatus(p.ID, model.StatusRented)
	require.NoError(t, err)

	err = svc.Delete(p.ID)
	require.ErrorIs(t, err, service.ErrProductRented)
}

func TestProductService_Delete_AllowsAvailableProduct(t *testing.T) {
	svc, db := newTestService(t)
	cat := model.Category{ID: uuid.New(), Name: "กล้อง"}
	require.NoError(t, db.Create(&cat).Error)
	p, err := svc.Create(cat.ID, "Canon EOS R5", "", 1200, "")
	require.NoError(t, err)

	require.NoError(t, svc.Delete(p.ID))
	_, err = svc.Get(p.ID)
	require.True(t, service.IsNotFound(err))
}
