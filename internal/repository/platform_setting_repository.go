package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

// PlatformSettingRepositoryInterface — xem model.PlatformSetting cho bối cảnh đầy đủ.
type PlatformSettingRepositoryInterface interface {
	// GetPlatformFeePercent trả về % phí nền tảng hiện hành. Chưa từng cấu hình (chưa có dòng
	// nào trong bảng) -> 0, ĐÚNG mặc định theo quyết định #2, không phải lỗi.
	GetPlatformFeePercent(ctx context.Context) (decimal.Decimal, error)
	SetPlatformFeePercent(ctx context.Context, percent decimal.Decimal, updatedBy uuid.UUID) error
}

type PlatformSettingRepository struct {
	db *gorm.DB
}

func NewPlatformSettingRepository(db *gorm.DB) *PlatformSettingRepository {
	return &PlatformSettingRepository{db: db}
}

func (r *PlatformSettingRepository) GetPlatformFeePercent(ctx context.Context) (decimal.Decimal, error) {
	var s model.PlatformSetting
	err := r.db.WithContext(ctx).Where("id = ?", model.SingletonSettingID).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	return s.PlatformFeePercent, nil
}

// SetPlatformFeePercent upsert dòng singleton — INSERT lần đầu, UPDATE các lần sau (ON CONFLICT
// theo khoá chính id cố định).
func (r *PlatformSettingRepository) SetPlatformFeePercent(ctx context.Context, percent decimal.Decimal, updatedBy uuid.UUID) error {
	s := &model.PlatformSetting{
		ID:                 model.SingletonSettingID,
		PlatformFeePercent: percent,
		UpdatedAt:          time.Now(),
		UpdatedBy:          &updatedBy,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"platform_fee_percent", "updated_at", "updated_by"}),
	}).Create(s).Error
}
