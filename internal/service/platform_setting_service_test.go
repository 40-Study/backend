package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// fakePlatformSettingRepo — fake trong-bộ-nhớ, không cần DB thật (test thuần logic validate %).
type fakePlatformSettingRepo struct {
	percent decimal.Decimal
}

func (f *fakePlatformSettingRepo) GetPlatformFeePercent(ctx context.Context) (decimal.Decimal, error) {
	return f.percent, nil
}

func (f *fakePlatformSettingRepo) SetPlatformFeePercent(ctx context.Context, percent decimal.Decimal, updatedBy uuid.UUID) error {
	f.percent = percent
	return nil
}

// Get/GetUpdaterName: bắt buộc để thoả interface sau khi mở rộng cho GET /admin/settings (phase 5);
// các test ở file này không dùng tới (hành vi thật được test ở handler/admin_settings_handler_test.go).
func (f *fakePlatformSettingRepo) Get(ctx context.Context) (*model.PlatformSetting, error) {
	return nil, nil
}

func (f *fakePlatformSettingRepo) GetUpdaterName(ctx context.Context, userID uuid.UUID) (string, error) {
	return "", nil
}

// TestSetPlatformFeePercent_RejectsOutOfRange (quyết định #2: % phải hợp lệ 0-100) — âm hoặc
// >100 là lỗi cấu hình rõ ràng (âm nghĩa là "phí âm" vô nghĩa, >100 nghĩa là thu nhiều hơn cả
// đơn — cả hai đều là input sai, không phải chính sách kinh doanh hợp lệ nào).
func TestSetPlatformFeePercent_RejectsOutOfRange(t *testing.T) {
	repo := &fakePlatformSettingRepo{}
	svc := NewPlatformSettingService(repo)

	cases := []struct {
		name    string
		percent decimal.Decimal
		wantErr bool
	}{
		{"0% hop le (mac dinh)", decimal.Zero, false},
		{"5% hop le", decimal.NewFromInt(5), false},
		{"100% bien tren hop le", decimal.NewFromInt(100), false},
		{"am khong hop le", decimal.NewFromInt(-1), true},
		{"tren 100 khong hop le", decimal.NewFromInt(101), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.SetPlatformFeePercent(context.Background(), uuid.New(), tc.percent)
			if tc.wantErr && !errors.Is(err, ErrInvalidPlatformFeePercent) {
				t.Fatalf("percent=%s: ky vong ErrInvalidPlatformFeePercent, duoc %v", tc.percent, err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("percent=%s: khong ky vong loi, duoc %v", tc.percent, err)
			}
		})
	}
}

// TestGetPlatformFeePercent_DefaultsToZero (quyết định #2): chưa từng cấu hình -> 0%, không phải
// lỗi — hành vi này thuộc về PlatformSettingRepository thật (GetRecordNotFound -> decimal.Zero),
// test ở đây chỉ xác nhận service KHÔNG tự ý đổi giá trị repo trả về.
func TestGetPlatformFeePercent_DefaultsToZero(t *testing.T) {
	repo := &fakePlatformSettingRepo{percent: decimal.Zero}
	svc := NewPlatformSettingService(repo)
	got, err := svc.GetPlatformFeePercent(context.Background())
	if err != nil {
		t.Fatalf("khong ky vong loi: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("got %s, want 0", got)
	}
}
