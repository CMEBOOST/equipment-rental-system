package repository

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/model"
)

type ProductRepo struct{ db *gorm.DB }

func NewProductRepo(db *gorm.DB) *ProductRepo { return &ProductRepo{db: db} }

// allowedSortColumns mirrors user-service/internal/repository/user_repo.go's
// allow-list: ORDER BY built from a raw query param must never pass an
// unvalidated column name through to SQL.
var allowedSortColumns = map[string]bool{
	"created_at":    true,
	"name":          true,
	"price_per_day": true,
}

type ProductFilter struct {
	Query       string // ค้นหาชื่อสินค้า (ILIKE %q%)
	CategoryID  *uuid.UUID
	Status      string
	Page, Limit int
	Sort, Order string
}

func (r *ProductRepo) Create(p *model.Product) error {
	return r.db.Create(p).Error
}

func (r *ProductRepo) FindByID(id uuid.UUID) (*model.Product, error) {
	var p model.Product
	if err := r.db.Preload("Category").First(&p, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProductRepo) List(f ProductFilter) ([]model.Product, int64, error) {
	q := r.db.Model(&model.Product{}).Preload("Category")
	if f.Query != "" {
		// LOWER(...) LIKE LOWER(?) rather than Postgres's ILIKE: this repo's
		// tests run against SQLite (glebarez/sqlite), which does not support
		// ILIKE at all ("syntax error"). The LOWER form gives the identical
		// case-insensitive match on both drivers, so production behavior on
		// Postgres is unchanged.
		q = q.Where("LOWER(name) LIKE LOWER(?)", "%"+f.Query+"%")
	}
	if f.CategoryID != nil {
		q = q.Where("category_id = ?", *f.CategoryID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sort := f.Sort
	if sort == "" || !allowedSortColumns[sort] {
		sort = "created_at"
	}
	order := strings.ToLower(strings.TrimSpace(f.Order))
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	page, limit := f.Page, f.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var products []model.Product
	// id is appended as a tiebreaker so paginated ordering is a total order —
	// same reasoning as UserRepo.List (rows sharing a sort value would
	// otherwise have an unspecified relative order across pages).
	err := q.Order(sort + " " + order + ", products.id " + order).
		Offset((page - 1) * limit).Limit(limit).Find(&products).Error
	return products, total, err
}

func (r *ProductRepo) Update(p *model.Product) error {
	return r.db.Save(p).Error
}

func (r *ProductRepo) UpdateStatus(id uuid.UUID, status string) (*model.Product, error) {
	p, err := r.FindByID(id)
	if err != nil {
		return nil, err
	}
	p.Status = status
	if err := r.db.Model(p).Update("status", status).Error; err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProductRepo) SoftDelete(id uuid.UUID) error {
	return r.db.Delete(&model.Product{}, "id = ?", id).Error
}
