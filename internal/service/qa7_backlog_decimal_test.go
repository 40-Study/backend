package service

// QA7 backlog (#155): các kiểm khoảng trên decimal.Decimal lấy từ body phải loại số mũ khổng lồ TRƯỚC khi so sánh (shopspring
// rescale 10^|exp|: `1e20000000` tốn ~2 s CPU mỗi phép so sánh). Mỗi ca đỏ khi bỏ phần chặn exponent ở: SetPlatformFeePercent,
// ContestService.validateUpsert (certificate_min_percentage), validateWithdrawalAmount.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
)

var qa7bHugeExponents = []string{"1e20000000", "1e-20000000", "0e20000000"}

func qa7bFast(t *testing.T, label string, fn func()) {
	t.Helper()
	start := time.Now()
	fn()
	// Chưa vá: mỗi lần so sánh ~2 s, trần 500 ms đủ để đỏ.
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("%s mất %v, muốn loại số mũ khổng lồ trước khi so sánh", label, elapsed)
	}
}

func TestQA7B_PlatformFeePercent_SoMuKhongLo_TuChoiNhanh(t *testing.T) {
	svc := NewPlatformSettingService(nil) // bị loại trước khi chạm repo; nil repo sẽ panic nếu validate lọt
	for _, in := range qa7bHugeExponents {
		qa7bFast(t, "SetPlatformFeePercent "+in, func() {
			if err := svc.SetPlatformFeePercent(context.Background(), uuid.New(), decimal.RequireFromString(in)); !errors.Is(err, ErrInvalidPlatformFeePercent) {
				t.Errorf("%s: err=%v, muốn ErrInvalidPlatformFeePercent", in, err)
			}
		})
	}
	for _, in := range []string{"-1", "100.5", "101"} { // hành vi cũ giữ nguyên
		if err := svc.SetPlatformFeePercent(context.Background(), uuid.New(), decimal.RequireFromString(in)); !errors.Is(err, ErrInvalidPlatformFeePercent) {
			t.Errorf("%s: err=%v, muốn ErrInvalidPlatformFeePercent", in, err)
		}
	}
}

// qa7bValidContestReq: mọi thứ hợp lệ trừ phần trăm chứng nhận, nên lỗi field certificate_min_percentage chỉ có thể đến từ
// chính phép kiểm đó. Request này đi hết lịch, giải, khoá học rồi dừng ở ErrContestQuizInvalid (quiz_id rỗng, chưa chạm repo).
func qa7bValidContestReq(p decimal.Decimal) *dto.ContestUpsertRequest {
	now := time.Now()
	return &dto.ContestUpsertRequest{
		Title: "QA7B contest", StartTime: now.Add(24 * time.Hour), EndTime: now.Add(48 * time.Hour),
		DurationMinutes: 60, CertificateMinPercentage: &p,
	}
}

func qa7bPercentageFieldError(err error) bool {
	var ve *ContestValidationError
	return errors.As(err, &ve) && len(ve.Errors) == 1 && ve.Errors[0].Field == "certificate_min_percentage"
}

func TestQA7B_ContestCertificateMinPercentage_SoMuKhongLo_TuChoiNhanh(t *testing.T) {
	svc := &ContestService{now: time.Now}
	validate := func(in string) error {
		return svc.validateUpsert(context.Background(), &ContestActor{}, qa7bValidContestReq(decimal.RequireFromString(in)), uuid.New(), nil)
	}
	for _, in := range append(append([]string{}, qa7bHugeExponents...), "-1", "100.01") {
		qa7bFast(t, "validateUpsert "+in, func() {
			if err := validate(in); !qa7bPercentageFieldError(err) {
				t.Errorf("%s: err=%v, muốn lỗi field certificate_min_percentage", in, err)
			}
		})
	}
	for _, in := range []string{"0", "55.5", "100"} { // hợp lệ: qua phép kiểm %, dừng ở bước sau
		if err := validate(in); qa7bPercentageFieldError(err) || !errors.Is(err, ErrContestQuizInvalid) {
			t.Errorf("%s: err=%v, muốn ErrContestQuizInvalid (đã qua phép kiểm %%)", in, err)
		}
	}
}

func TestQA7B_WithdrawalAmount_SoMuKhongLo_TuChoiNhanh(t *testing.T) {
	min := decimal.NewFromInt(100000)
	for _, in := range qa7bHugeExponents {
		qa7bFast(t, "validateWithdrawalAmount "+in, func() {
			if err := validateWithdrawalAmount(decimal.RequireFromString(in), min); !errors.Is(err, ErrWithdrawalInvalidAmount) {
				t.Errorf("%s: err=%v, muốn ErrWithdrawalInvalidAmount", in, err)
			}
		})
	}
	if err := validateWithdrawalAmount(decimal.NewFromInt(500000), min); err != nil { // số hợp lệ vẫn qua
		t.Errorf("500000: err=%v", err)
	}
}
