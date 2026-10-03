package dto

import (
	"strings"
	"testing"

	"study.com/v1/internal/utils"
)

func ptrF(v float64) *float64 { return &v }
func ptrI(v int32) *int32     { return &v }

// B-21 (QA hồi quy 03/10): voucher "giảm 0đ" từng được lưu rồi xuất hiện ở /vouchers/public. Mức giảm
// phải > 0 ở cả tạo lẫn sửa; thông báo lỗi không được nói "0 characters" cho trường số.
func TestVoucherRequests_RejectZeroDiscount(t *testing.T) {
	base := CreateVoucherRequest{Code: "ZERO-OFF", Name: "Zero off", DiscountUnit: "MONEY", DiscountMethod: "FIXED"}

	cases := []struct {
		name  string
		field string
		mut   func(r *CreateVoucherRequest)
	}{
		{"tiền cố định = 0", "discount_amount_money", func(r *CreateVoucherRequest) { r.DiscountAmountMoney = ptrF(0) }},
		{"điểm cố định = 0", "discount_amount_points", func(r *CreateVoucherRequest) { r.DiscountAmountPoints = ptrI(0) }},
		{"phần trăm = 0", "discount_percent", func(r *CreateVoucherRequest) { r.DiscountPercent = ptrF(0) }},
	}
	for _, c := range cases {
		t.Run("tạo: "+c.name, func(t *testing.T) {
			req := base
			c.mut(&req)
			errs := utils.ValidateStruct(req)
			if len(errs) != 1 || errs[0].Field != c.field {
				t.Fatalf("muốn đúng 1 lỗi ở %s, được %+v", c.field, errs)
			}
			if strings.Contains(errs[0].Message, "characters") {
				t.Fatalf("thông báo cho trường số không được nói characters: %q", errs[0].Message)
			}
		})
	}

	t.Run("sửa: tiền cố định = 0 bị từ chối", func(t *testing.T) {
		errs := utils.ValidateStruct(UpdateVoucherRequest{DiscountAmountMoney: ptrF(0)})
		if len(errs) != 1 || errs[0].Field != "discount_amount_money" {
			t.Fatalf("muốn lỗi ở discount_amount_money, được %+v", errs)
		}
	})

	t.Run("giá trị dương và vắng mặt vẫn hợp lệ", func(t *testing.T) {
		ok := base
		ok.DiscountAmountMoney = ptrF(50000)
		if errs := utils.ValidateStruct(ok); len(errs) != 0 {
			t.Fatalf("voucher hợp lệ bị từ chối: %+v", errs)
		}
		if errs := utils.ValidateStruct(UpdateVoucherRequest{}); len(errs) != 0 {
			t.Fatalf("PUT rỗng phải hợp lệ: %+v", errs)
		}
	})
}
