package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

type LoginLogRepo struct{ db *gorm.DB }

func NewLoginLogRepo(db *gorm.DB) *LoginLogRepo { return &LoginLogRepo{db: db} }

func (r *LoginLogRepo) Create(l *model.LoginLog) error {
	return r.db.Create(l).Error
}

// ListForUser returns one page of a user's login history, newest first.
//
// success is the optional `success=true|false` filter from design doc
// §7.9/§7.17: nil means "no filter" (both successful and failed attempts),
// which is what an absent query parameter maps to. It is applied to the
// Count as well as the Find, so meta.total describes the filtered set rather
// than the whole history.
func (r *LoginLogRepo) ListForUser(userID uuid.UUID, success *bool, page, limit int) ([]model.LoginLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := r.db.Model(&model.LoginLog{}).Where("user_id = ?", userID)
	if success != nil {
		q = q.Where("success = ?", *success)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.LoginLog
	// id is appended as a tiebreaker for the same reason as UserRepo.List:
	// login_logs.created_at is not unique (a failed and a successful attempt
	// can land in the same clock tick), and without a total order paginated
	// results are non-deterministic. id is a BIGSERIAL, so "created_at desc,
	// id desc" is exactly reverse insertion order.
	err := q.Order("created_at desc, id desc").Offset((page - 1) * limit).Limit(limit).Find(&logs).Error
	return logs, total, err
}
