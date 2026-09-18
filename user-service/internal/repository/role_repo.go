package repository

import (
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

type RoleRepo struct{ db *gorm.DB }

func NewRoleRepo(db *gorm.DB) *RoleRepo { return &RoleRepo{db: db} }

func (r *RoleRepo) FindByName(name string) (*model.Role, error) {
	var role model.Role
	if err := r.db.Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *RoleRepo) List() ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Order("id").Find(&roles).Error
	return roles, err
}
