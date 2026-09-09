package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// TestCalculateVoucherDiscountDecimal (item 24 vòng 2b, H2-01/H2-02 review vòng 3) — pin lại
// công thức tính discount PHẢI khớp calculateVoucherDiscount phía web
// (web/src/services/voucher.service.ts): discount_unit phải MONEY cho CẢ HAI method (không
// chỉ FIXED), Math.floor(discount) TRƯỚC khi clamp về subtotal/0, max_discount_money chỉ áp
// khi > 0. Các case dưới đây đối chiếu TRỰC TIẾP với logic web đã đọc trong review vòng 3.
func TestCalculateVoucherDiscountDecimal(t *testing.T) {
	money := func(v string) *decimal.Decimal {
		d := decimal.RequireFromString(v)
		return &d
	}
	percent := func(v string) *decimal.Decimal {
		d := decimal.RequireFromString(v)
		return &d
	}

	tests := []struct {
		name     string
		voucher  *model.Voucher
		subtotal decimal.Decimal
		want     string
	}{
		{
			name: "FIXED MONEY - giam dung so tien",
			voucher: &model.Voucher{
				DiscountMethod:      model.DiscountMethodFixed,
				DiscountUnit:        model.DiscountUnitMoney,
				DiscountAmountMoney: money("50000"),
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "50000",
		},
		{
			name: "FIXED MONEY vuot subtotal -> clamp ve subtotal",
			voucher: &model.Voucher{
				DiscountMethod:      model.DiscountMethodFixed,
				DiscountUnit:        model.DiscountUnitMoney,
				DiscountAmountMoney: money("500000"),
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "200000",
		},
		{
			name: "PERCENT MONEY khong vuot max",
			voucher: &model.Voucher{
				DiscountMethod:   model.DiscountMethodPercent,
				DiscountUnit:     model.DiscountUnitMoney,
				DiscountPercent:  percent("10"),
				MaxDiscountMoney: money("100000"),
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "20000", // 10% of 200000 = 20000, under cap
		},
		{
			name: "PERCENT MONEY vuot max_discount_money -> clamp",
			voucher: &model.Voucher{
				DiscountMethod:   model.DiscountMethodPercent,
				DiscountUnit:     model.DiscountUnitMoney,
				DiscountPercent:  percent("50"),
				MaxDiscountMoney: money("30000"),
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "30000", // 50% of 200000 = 100000, capped to 30000
		},
		{
			name: "H2-01: PERCENT + DiscountUnitPoint -> 0 (khong duoc ap dung, khop web)",
			voucher: &model.Voucher{
				DiscountMethod:   model.DiscountMethodPercent,
				DiscountUnit:     model.DiscountUnitPoint,
				DiscountPercent:  percent("50"),
				MaxDiscountMoney: money("1000000"),
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "0", // truoc H2-01: se ra 100000 (SAI, web tu choi hoan toan)
		},
		{
			name: "DiscountUnitPoint (FIXED) khong ap dung cho don hang tien mat -> 0",
			voucher: &model.Voucher{
				DiscountMethod:       model.DiscountMethodFixed,
				DiscountUnit:         model.DiscountUnitPoint,
				DiscountAmountPoints: 100,
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "0",
		},
		{
			name: "H2-02: PERCENT tren subtotal le -> Floor truoc khi clamp",
			voucher: &model.Voucher{
				DiscountMethod:  model.DiscountMethodPercent,
				DiscountUnit:    model.DiscountUnitMoney,
				DiscountPercent: percent("10"),
			},
			subtotal: decimal.RequireFromString("199999"),
			want:     "19999", // 199999 * 10% = 19999.9 -> floor = 19999 (khong phai 19999.9)
		},
		{
			name: "max_discount_money = 0 (khong gioi han) -> khong clamp",
			voucher: &model.Voucher{
				DiscountMethod:   model.DiscountMethodPercent,
				DiscountUnit:     model.DiscountUnitMoney,
				DiscountPercent:  percent("50"),
				MaxDiscountMoney: money("0"),
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "100000", // cap=0 nghia la khong gioi han, khop web "if (cap > 0)"
		},
		{
			// H3-03 (review vòng 4): ví dụ THẬT trong báo cáo review — PERCENT 10% trên
			// subtotal=5 (voucher DiscountUnit=MONEY, tức KHÔNG bị chặn bởi check H2-01 ở
			// ValidateAndApplyVoucher như case POINT phía trên) -> 0.5 -> Floor -> 0. Trước H3-03,
			// ValidateAndApplyVoucher vẫn trả (voucher, 0, nil) = "áp dụng thành công" cho case
			// này và CreateOrder gọi ReserveVoucherUsage, tiêu một lượt used_count thật cho một
			// mã không giảm được đồng nào. Hàm calculateVoucherDiscountDecimal ở đây chỉ pin lại
			// discount=0 LÀ ĐÚNG (không đổi công thức) — phần TỪ CHỐI (ErrVoucherNotApplicable)
			// nằm trong ValidateAndApplyVoucher, hàm cần DB thật (vs.vr) nên không có unit test
			// độc lập ở file này (xem "chưa làm" trong báo cáo vòng 4).
			name: "H3-03: PERCENT MONEY tren subtotal qua nho -> discount=0 (ValidateAndApplyVoucher se tu choi)",
			voucher: &model.Voucher{
				DiscountMethod:  model.DiscountMethodPercent,
				DiscountUnit:    model.DiscountUnitMoney,
				DiscountPercent: percent("10"),
			},
			subtotal: decimal.RequireFromString("5"),
			want:     "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateVoucherDiscountDecimal(tt.voucher, tt.subtotal)
			want := decimal.RequireFromString(tt.want)
			if !got.Equal(want) {
				t.Errorf("calculateVoucherDiscountDecimal() = %s, want %s", got.String(), want.String())
			}
		})
	}
}
