package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Review Phase 4 (MINOR): phân bổ phí nền tảng theo từng khoá rồi làm tròn 2 chữ số từng dòng có
// thể lệch 1–2 xu so với phí cả đơn. Đơn 3 khoá của 3 giảng viên, giá lẻ, phí 84.701đ:
// 84701×199000/847000 = 19900,2349 → 19900,23; ×299000 → 29900,3530 → 29900,35;
// ×349000 → 34900,4120 → 34900,41; cộng lại 84700,99 (thiếu 1 xu). Phần dư phải dồn vào 1 dòng để
// Σ phần GV + phí đơn = tổng đơn, ĐÚNG TỪNG XU. ĐỎ với công thức làm tròn độc lập từng dòng cũ.
func TestTeacherEarnings_FeeRoundingRemainderBalancesOrderTotal(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	prices := []int64{199000, 299000, 349000}
	const fee, total = 84701, 847000

	teachers := make([]uuid.UUID, len(prices))
	lines := make([]orderLine, len(prices))
	for i, p := range prices {
		teachers[i] = f.teacher(true)
		lines[i] = orderLine{f.course(teachers[i]), p}
	}
	f.order("completed", fee, lines...)

	sum := decimal.NewFromInt(fee)
	for i, id := range teachers {
		w, err := f.wallet.GetTeacherWallet(ctx, id)
		if err != nil {
			t.Fatalf("GetTeacherWallet: %v", err)
		}
		// Mỗi phần GV lệch tối đa 0,01 so với tỉ lệ chính xác.
		price := decimal.NewFromInt(prices[i])
		exact := price.Sub(decimal.NewFromInt(fee).Mul(price).Div(decimal.NewFromInt(total)))
		if w.TotalEarnings.Sub(exact).Abs().GreaterThan(decimal.RequireFromString("0.01")) {
			t.Fatalf("phần GV %d = %s, lệch quá 0,01 so với %s", i, w.TotalEarnings, exact.StringFixed(4))
		}
		sum = sum.Add(w.TotalEarnings)
	}
	if !sum.Equal(decimal.NewFromInt(total)) {
		t.Fatalf("Σ phần GV + phí = %s, muốn đúng %d (tổng đơn)", sum, total)
	}
}
