package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/model"
)

type CategoryRepo struct{ db *gorm.DB }

func NewCategoryRepo(db *gorm.DB) *CategoryRepo { return &CategoryRepo{db: db} }

func (r *CategoryRepo) List() ([]model.Category, error) {
	var categories []model.Category
	err := r.db.Order("name asc").Find(&categories).Error
	return categories, err
}

func (r *CategoryRepo) FindByID(id uuid.UUID) (*model.Category, error) {
	var c model.Category
	if err := r.db.First(&c, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// Exists reports whether a category with this id exists — used by
// ProductRepo/ProductService to validate CategoryID on create/update without
// pulling the full row.
func (r *CategoryRepo) Exists(id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&model.Category{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}
