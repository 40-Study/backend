package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

// BlockRow — một dòng danh sách người đã chặn.
type BlockRow struct {
	FriendUserRow
	CreatedAt time.Time `gorm:"column:created_at"`
}

type UserBlockRepository struct {
	db *gorm.DB
}

func NewUserBlockRepository(db *gorm.DB) *UserBlockRepository {
	return &UserBlockRepository{db: db}
}

// WithTx trả bản repository dùng chung transaction `tx` (để chặn + xoá bạn nằm cùng một transaction).
func (r *UserBlockRepository) WithTx(tx *FriendshipRepository) *UserBlockRepository {
	return &UserBlockRepository{db: tx.db}
}

// Create ghi block; đã chặn rồi thì không làm gì (idempotent, không lỗi).
func (r *UserBlockRepository) Create(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	b := model.UserBlock{BlockerID: blockerID, BlockedID: blockedID}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&b).Error
}

func (r *UserBlockRepository) Delete(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("blocker_id = ? AND blocked_id = ?", blockerID, blockedID).Delete(&model.UserBlock{}).Error
}

// IsBlockedBy — blocker có đang chặn blocked không (một chiều).
func (r *UserBlockRepository) IsBlockedBy(ctx context.Context, blockerID, blockedID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.UserBlock{}).
		Where("blocker_id = ? AND blocked_id = ?", blockerID, blockedID).Count(&n).Error
	return n > 0, err
}

// IsBlockedEitherWay — có block ở BẤT KỲ chiều nào giữa a và b.
func (r *UserBlockRepository) IsBlockedEitherWay(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.UserBlock{}).
		Where("(blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)", a, b, b, a).
		Count(&n).Error
	return n > 0, err
}

func (r *UserBlockRepository) List(ctx context.Context, blockerID uuid.UUID, page, limit int) ([]BlockRow, int64, error) {
	base := r.db.WithContext(ctx).Table("user_blocks b").
		Joins("JOIN users u ON u.id = b.blocked_id").
		Where("b.blocker_id = ? AND u.deleted_at IS NULL", blockerID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []BlockRow
	err := base.Select(friendUserColumns + ", b.created_at AS created_at").
		Order("b.created_at DESC, b.id ASC").Offset((page - 1) * limit).Limit(limit).Scan(&rows).Error
	return rows, total, err
}
