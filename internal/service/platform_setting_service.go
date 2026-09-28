package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/repository"
)

// ErrInvalidPlatformFeePercent — % phải nằm trong [0, 100].
var ErrInvalidPlatformFeePercent = errors.New("platform fee percent must be between 0 and 100")

// PlatformSettingServiceInterface — quyết định #2 (27/09/2026): % phí nền tảng cấu hình được,
// mặc định 0%.
type PlatformSettingServiceInterface interface {
	GetPlatformFeePercent(ctx context.Context) (decimal.Decimal, error)
	SetPlatformFeePercent(ctx context.Context, actorID uuid.UUID, percent decimal.Decimal) error
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
