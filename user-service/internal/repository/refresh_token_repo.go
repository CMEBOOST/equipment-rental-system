package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

type RefreshTokenRepo struct{ db *gorm.DB }

func NewRefreshTokenRepo(db *gorm.DB) *RefreshTokenRepo { return &RefreshTokenRepo{db: db} }

func (r *RefreshTokenRepo) Create(t *model.RefreshToken) error {
	return r.db.Create(t).Error
}

func (r *RefreshTokenRepo) FindByHash(hash string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	if err := r.db.Where("token_hash = ?", hash).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *RefreshTokenRepo) RevokeByHash(hash string) error {
	return r.db.Model(&model.RefreshToken{}).Where("token_hash = ?", hash).
		Update("revoked_at", time.Now()).Error
}

func (r *RefreshTokenRepo) RevokeAllForUser(userID uuid.UUID) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", time.Now()).Error
}

func (r *RefreshTokenRepo) ListActiveForUser(userID uuid.UUID) ([]model.RefreshToken, error) {
	var tokens []model.RefreshToken
	err := r.db.Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, time.Now()).
		Find(&tokens).Error
	return tokens, err
}
