package repository

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

type UserRepo struct{ db *gorm.DB }

// allowedSortColumns defines the columns that can be safely used in ORDER BY clauses
var allowedSortColumns = map[string]bool{
	"created_at": true,
	"email":      true,
	"username":   true,
	"full_name":  true,
}

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

type UserFilter struct {
	Query    string
	Role     string
	IsActive *bool
	Page, Limit int
	Sort, Order string
}

func (r *UserRepo) Create(u *model.User) error {
	return r.db.Create(u).Error
}

func (r *UserRepo) FindByEmail(email string) (*model.User, error) {
	var u model.User
	if err := r.db.Preload("Role").Where("email = ?", email).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) FindByUsername(username string) (*model.User, error) {
	var u model.User
	if err := r.db.Preload("Role").Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) FindByID(id uuid.UUID) (*model.User, error) {
	var u model.User
	if err := r.db.Preload("Role").First(&u, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) List(f UserFilter) ([]model.User, int64, error) {
	q := r.db.Model(&model.User{}).Preload("Role")
	if f.Query != "" {
		q = q.Where("email ILIKE ? OR username ILIKE ? OR full_name ILIKE ?", "%"+f.Query+"%", "%"+f.Query+"%", "%"+f.Query+"%")
	}
	if f.Role != "" {
		q = q.Joins("JOIN roles ON roles.id = users.role_id").Where("roles.name = ?", f.Role)
	}
	if f.IsActive != nil {
		q = q.Where("is_active = ?", *f.IsActive)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Validate and sanitize sort column (prevent SQL injection)
	sort := f.Sort
	if sort == "" || !allowedSortColumns[sort] {
		sort = "created_at"
	}

	// Validate and sanitize order direction (prevent SQL injection)
	order := strings.ToLower(strings.TrimSpace(f.Order))
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	// Validate pagination parameters
	page, limit := f.Page, f.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var users []model.User
	err := q.Order(sort + " " + order).Offset((page - 1) * limit).Limit(limit).Find(&users).Error
	return users, total, err
}

func (r *UserRepo) Update(u *model.User) error {
	return r.db.Save(u).Error
}

func (r *UserRepo) SoftDelete(id uuid.UUID) error {
	return r.db.Delete(&model.User{}, "id = ?", id).Error
}
