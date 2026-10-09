package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

// ErrInvalidPlatformFeePercent — % phải nằm trong [0, 100].
var ErrInvalidPlatformFeePercent = errors.New("platform fee percent must be between 0 and 100")

// PlatformSettingServiceInterface — quyết định #2 (27/09/2026): % phí nền tảng cấu hình được,
// mặc định 0%.
type PlatformSettingServiceInterface interface {
	GetPlatformFeePercent(ctx context.Context) (decimal.Decimal, error)
	SetPlatformFeePercent(ctx context.Context, actorID uuid.UUID, percent decimal.Decimal) error
	// GetSettings — toàn bộ cấu hình cho trang /admin/settings (contract C4).
	GetSettings(ctx context.Context) (*dto.AdminSettingsDTO, error)
}

type PlatformSettingService struct {
	repo repository.PlatformSettingRepositoryInterface
}

func NewPlatformSettingService(repo repository.PlatformSettingRepositoryInterface) *PlatformSettingService {
	return &PlatformSettingService{repo: repo}
}

func (s *PlatformSettingService) GetPlatformFeePercent(ctx context.Context) (decimal.Decimal, error) {
	return s.repo.GetPlatformFeePercent(ctx)
}

func (s *PlatformSettingService) SetPlatformFeePercent(ctx context.Context, actorID uuid.UUID, percent decimal.Decimal) error {
	if percent.LessThan(decimal.Zero) || percent.GreaterThan(decimal.NewFromInt(100)) {
		return ErrInvalidPlatformFeePercent
	}
	return s.repo.SetPlatformFeePercent(ctx, percent, actorID)
}

// GetSettings đọc dòng singleton + tên người cập nhật. Chưa từng cấu hình: phí 0, updated_at và
// updated_by null (mặc định theo quyết định #2). Khi thêm tham số cấu hình mới, thêm trường vào
// dto.AdminSettingsDTO và điền ở đây.
func (s *PlatformSettingService) GetSettings(ctx context.Context) (*dto.AdminSettingsDTO, error) {
	row, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return &dto.AdminSettingsDTO{PlatformFeePercent: decimal.Zero}, nil
	}
	out := &dto.AdminSettingsDTO{PlatformFeePercent: row.PlatformFeePercent}
	// Dòng được tạo bởi SetPlatformFeePercent nên luôn có updated_by; dòng cũ/thủ công thì không
	// có -> updated_at vẫn trả, updated_by null.
	updatedAt := row.UpdatedAt
	out.UpdatedAt = &updatedAt
	if row.UpdatedBy != nil {
		name, err := s.repo.GetUpdaterName(ctx, *row.UpdatedBy)
		if err != nil {
			return nil, err
		}
		out.UpdatedBy = &dto.AdminSettingsUpdaterDTO{ID: *row.UpdatedBy, Name: name}
	}
	return out, nil
}
