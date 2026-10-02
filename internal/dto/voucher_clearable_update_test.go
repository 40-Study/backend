package dto

import (
	"encoding/json"
	"testing"
)

// L6 mục 8: PUT /vouchers/:id phải xoá được ngày bắt đầu/kết thúc và trần giảm bằng null (hoặc "").
// Vắng mặt = giữ nguyên; có giá trị = đặt mới. Bỏ UnmarshalJSON của UpdateVoucherRequest thì cờ
// *Cleared không bao giờ bật và các ca "xoá" ĐỎ.
func TestUpdateVoucherRequest_ClearableFieldsTriState(t *testing.T) {
	t.Run("vắng mặt = giữ nguyên", func(t *testing.T) {
		var r UpdateVoucherRequest
		if err := json.Unmarshal([]byte(`{"name":"Voucher A"}`), &r); err != nil {
			t.Fatal(err)
		}
		if r.StartDate != nil || r.EndDate != nil || r.MaxDiscountMoney != nil || r.MaxDiscountPoints != nil ||
			r.StartDateCleared || r.EndDateCleared || r.MaxDiscountMoneyCleared || r.MaxDiscountPointsCleared {
			t.Fatalf("không gửi gì mà có giá trị/cờ xoá: %+v", r)
		}
		if r.Name == nil || *r.Name != "Voucher A" {
			t.Fatalf("trường khác bị mất: %+v", r)
		}
	})
	t.Run("null và chuỗi rỗng = xoá cả bốn trường", func(t *testing.T) {
		for _, body := range []string{
			`{"start_date":null,"end_date":null,"max_discount_money":null,"max_discount_points":null}`,
			`{"start_date":"","end_date":"","max_discount_money":"","max_discount_points":""}`,
		} {
			var r UpdateVoucherRequest
			if err := json.Unmarshal([]byte(body), &r); err != nil {
				t.Fatalf("%s: %v", body, err)
			}
			if !r.StartDateCleared || !r.EndDateCleared || !r.MaxDiscountMoneyCleared || !r.MaxDiscountPointsCleared {
				t.Fatalf("%s: cờ xoá = %+v, muốn cả bốn bật", body, r)
			}
			if r.StartDate != nil || r.EndDate != nil || r.MaxDiscountMoney != nil || r.MaxDiscountPoints != nil {
				t.Fatalf("%s: xoá mà vẫn còn giá trị: %+v", body, r)
			}
		}
	})
	t.Run("có giá trị = đặt mới", func(t *testing.T) {
		var r UpdateVoucherRequest
		body := `{"start_date":"2026-10-01T00:00:00Z","end_date":"2026-12-31T00:00:00Z","max_discount_money":50000,"max_discount_points":120}`
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatal(err)
		}
		if r.StartDate == nil || r.EndDate == nil || r.MaxDiscountMoney == nil || *r.MaxDiscountMoney != 50000 ||
			r.MaxDiscountPoints == nil || *r.MaxDiscountPoints != 120 {
			t.Fatalf("giá trị bị mất: %+v", r)
		}
		if r.StartDateCleared || r.EndDateCleared || r.MaxDiscountMoneyCleared || r.MaxDiscountPointsCleared {
			t.Fatalf("có giá trị mà bật cờ xoá: %+v", r)
		}
	})
	t.Run("sai kiểu bị từ chối", func(t *testing.T) {
		if err := json.Unmarshal([]byte(`{"max_discount_money":"abc"}`), &UpdateVoucherRequest{}); err == nil {
			t.Fatal("max_discount_money kiểu chuỗi không phải số phải bị từ chối")
		}
	})
}
