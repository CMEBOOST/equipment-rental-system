package repository

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/model"
)

// ErrStatusConflict is returned by UpdateStatus when the caller asked to
// transition a product to "rented" but its current status was not
// "available" — see the compare-and-swap below and CONTRACT.md §8.2's
// atomicity requirement (added specifically to close the race where two
// concurrent rental requests could both "win" and double-book one product).
var ErrStatusConflict = errors.New("product status is not available")

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
	if status != model.StatusRented {
		// Every other transition (→ available, → maintenance) is intentionally
		// unconditional per CONTRACT.md §8.2 — idempotent by design, so a retry
		// or an admin correcting an already-correct status never 409s.
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

	// → rented is the one direction with a real race: two callers could both
	// read "available" and both try to rent the same product. The WHERE
	// clause makes the flip atomic at the database level — only the caller
	// whose UPDATE runs while the row is still "available" affects a row.
	// .Model(&model.Product{}) keeps GORM's automatic soft-delete scope
	// (deleted_at IS NULL), so a deleted product behaves like "not found"
	// below rather than surfacing as a status conflict.
	result := r.db.Model(&model.Product{}).
		Where("id = ? AND status = ?", id, model.StatusAvailable).
		Update("status", model.StatusRented)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		if _, err := r.FindByID(id); err != nil {
			return nil, err // not found (or soft-deleted) — propagate gorm.ErrRecordNotFound
		}
		return nil, ErrStatusConflict // exists, but wasn't "available"
	}
	return r.FindByID(id)
}

func (r *ProductRepo) SoftDelete(id uuid.UUID) error {
	return r.db.Delete(&model.Product{}, "id = ?", id).Error
}
