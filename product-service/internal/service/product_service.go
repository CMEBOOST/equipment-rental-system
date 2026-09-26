// Package service holds business rules that sit above plain CRUD: validating
// a referenced category exists, applying partial updates, and refusing to
// delete a product that is currently out on rent. Mirrors the
// handler -> service -> repository layering used throughout user-service.
package service

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/model"
	"github.com/equipment-rental-system/product-service/internal/repository"
)

var (
	// ErrCategoryNotFound is returned by Create/Update when category_id does
	// not reference an existing row. It is checked here (a query) rather
	// than left to the DB's FK constraint because a raw FK-violation error
	// is driver-specific and harder to map to VALIDATION_ERROR than a
	// dedicated sentinel.
	ErrCategoryNotFound = errors.New("category not found")
	// ErrProductRented is returned by Delete when the product's status is
	// "rented" — deleting it would orphan whatever rental-service record
	// points at it (CONTRACT.md §3 forbids a cross-DB FK to enforce this for
	// us, so product-service enforces it locally instead).
	ErrProductRented = errors.New("product is currently rented")
)

type ProductService struct {
	products   *repository.ProductRepo
	categories *repository.CategoryRepo
}

func NewProductService(products *repository.ProductRepo, categories *repository.CategoryRepo) *ProductService {
	return &ProductService{products: products, categories: categories}
}

func (s *ProductService) Create(categoryID uuid.UUID, name, description string, pricePerDay float64, imageURL string) (*model.Product, error) {
	ok, err := s.categories.Exists(categoryID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrCategoryNotFound
	}

	p := &model.Product{
		ID:          uuid.New(),
		CategoryID:  categoryID,
		Name:        name,
		Description: description,
		PricePerDay: pricePerDay,
		Status:      model.StatusAvailable,
		ImageURL:    imageURL,
	}
	if err := s.products.Create(p); err != nil {
		return nil, err
	}
	// Create leaves p.Category zero-valued (no round trip); reload so the
	// handler's response can include the category name like every other
	// endpoint does.
	return s.products.FindByID(p.ID)
}

func (s *ProductService) Get(id uuid.UUID) (*model.Product, error) {
	return s.products.FindByID(id)
}

func (s *ProductService) List(f repository.ProductFilter) ([]model.Product, int64, error) {
	return s.products.List(f)
}

// Update applies only the fields the caller sent (nil pointer = "leave
// unchanged"), mirroring UserService.UpdateUser's partial-update shape.
func (s *ProductService) Update(id uuid.UUID, categoryID *uuid.UUID, name, description, imageURL *string, pricePerDay *float64) (*model.Product, error) {
	p, err := s.products.FindByID(id)
	if err != nil {
		return nil, err
	}

	if categoryID != nil {
		ok, err := s.categories.Exists(*categoryID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrCategoryNotFound
		}
		p.CategoryID = *categoryID
	}
	if name != nil {
		p.Name = *name
	}
	if description != nil {
		p.Description = *description
	}
	if imageURL != nil {
		p.ImageURL = *imageURL
	}
	if pricePerDay != nil {
		p.PricePerDay = *pricePerDay
	}

	if err := s.products.Update(p); err != nil {
		return nil, err
	}
	return s.products.FindByID(id)
}

// ChangeStatus is called both from an admin/staff HTTP request and from
// rental-service's internal call (CONTRACT.md §8.2 — "Admin, Staff,
// Internal"). DTO validation (binding:"oneof=...") already restricts the
// value before it reaches here for the HTTP path; the internal path
// validates it itself (see handler.ProductHandler.InternalChangeStatus).
func (s *ProductService) ChangeStatus(id uuid.UUID, status string) (*model.Product, error) {
	return s.products.UpdateStatus(id, status)
}

// Delete refuses to remove a product that is currently rented (see
// ErrProductRented) — check-then-delete rather than a DB constraint because
// there is no cross-service FK to lean on (CONTRACT.md §3).
func (s *ProductService) Delete(id uuid.UUID) error {
	p, err := s.products.FindByID(id)
	if err != nil {
		return err
	}
	if p.Status == model.StatusRented {
		return ErrProductRented
	}
	return s.products.SoftDelete(id)
}

// IsNotFound is a small helper so handlers can share the same
// gorm.ErrRecordNotFound check without importing gorm directly.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
