package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// TestCalculateVoucherDiscountDecimal (item 24, review web vòng 1) — pin lại công thức tính
// discount PHẢI khớp calculateVoucherDiscount phía web (voucher-input.tsx): dùng
// discount_unit/discount_method/max_discount_money, clamp về subtotal, không âm.
func TestCalculateVoucherDiscountDecimal(t *testing.T) {
	money := func(v string) *decimal.Decimal {
		d := decimal.RequireFromString(v)
		return &d
	}

	tests := []struct {
		name    string
		voucher *model.Voucher
		subtotal decimal.Decimal
		want    string
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
			name: "PERCENT khong vuot max",
			voucher: &model.Voucher{
				DiscountMethod: model.DiscountMethodPercent,
				DiscountPercent: func() *decimal.Decimal {
					d := decimal.RequireFromString("10")
					return &d
				}(),
				MaxDiscountMoney: money("100000"),
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "20000", // 10% of 200000 = 20000, under cap
		},
		{
			name: "PERCENT vuot max_discount_money -> clamp",
			voucher: &model.Voucher{
				DiscountMethod: model.DiscountMethodPercent,
				DiscountPercent: func() *decimal.Decimal {
					d := decimal.RequireFromString("50")
					return &d
				}(),
				MaxDiscountMoney: money("30000"),
			},
			subtotal: decimal.RequireFromString("200000"),
			want:     "30000", // 50% of 200000 = 100000, capped to 30000
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
