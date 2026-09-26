package repository

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/rental-service/internal/model"
)

type RentalRepository struct{ db *gorm.DB }

type RentalFilter struct {
	UserID string
	Status string
	Page   int
	Limit  int
	Sort   string
	Order  string
}

func NewRentalRepository(db *gorm.DB) *RentalRepository { return &RentalRepository{db: db} }

func (r *RentalRepository) Create(rental *model.Rental) error { return r.db.Create(rental).Error }

func (r *RentalRepository) Get(id uuid.UUID) (*model.Rental, error) {
	var rental model.Rental
	if err := r.db.First(&rental, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &rental, nil
}

func (r *RentalRepository) Update(rental *model.Rental) error { return r.db.Save(rental).Error }

func (r *RentalRepository) List(filter RentalFilter) ([]model.Rental, int64, error) {
	q := r.db.Model(&model.Rental{})
	if filter.UserID != "" {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	sort := filter.Sort
	if sort != "created_at" && sort != "start_date" && sort != "due_date" {
		sort = "created_at"
	}
	order := strings.ToLower(filter.Order)
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	var rentals []model.Rental
	err := q.Order(sort + " " + order + ", id " + order).Offset((page - 1) * limit).Limit(limit).Find(&rentals).Error
	return rentals, total, err
}
