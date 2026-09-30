package dto

import (
	"encoding/json"
	"testing"
)

// Lane U (UX-9): trước đây `null`/"" và "không gửi" đều ra nil nên không xoá được SĐT hay giá
// khuyến mãi. Các test dưới pin cả ba trạng thái; bỏ UnmarshalJSON -> cờ *Cleared không bao giờ
// bật và các ca "xoá" ĐỎ.

func TestUpdateMeRequestDto_PhoneTriState(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantCleared bool
		wantPhone   *string
	}{
		{"vắng mặt = giữ nguyên", `{"bio":"x"}`, false, nil},
		{"null = xoá", `{"phone":null}`, true, nil},
		{"chuỗi rỗng = xoá", `{"phone":""}`, true, nil},
		{"có số = đặt số mới", `{"phone":"+84901234567"}`, false, strPtr("+84901234567")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req UpdateMeRequestDto
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if req.PhoneCleared != tc.wantCleared {
				t.Fatalf("PhoneCleared = %v, muốn %v", req.PhoneCleared, tc.wantCleared)
			}
			if (req.Phone == nil) != (tc.wantPhone == nil) || (req.Phone != nil && *req.Phone != *tc.wantPhone) {
				t.Fatalf("Phone = %v, muốn %v", req.Phone, tc.wantPhone)
			}
		})
	}
}

func TestUpdateMeRequestDto_OtherFieldsStillParsedAndBadPhoneTypeRejected(t *testing.T) {
	var req UpdateMeRequestDto
	if err := json.Unmarshal([]byte(`{"username":"abc123","full_name":"Nguyen Van A","phone":null}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Username == nil || *req.Username != "abc123" || req.FullName == nil || *req.FullName != "Nguyen Van A" {
		t.Fatalf("các trường khác bị mất: %+v", req)
	}
	if err := json.Unmarshal([]byte(`{"phone":123}`), &UpdateMeRequestDto{}); err == nil {
		t.Fatal("phone kiểu số phải bị từ chối, không được lặng lẽ bỏ qua")
	}
}

func TestUpdateCourseDTO_DiscountPriceTriState(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantCleared bool
		wantPrice   string // "" = nil
	}{
		{"vắng mặt = giữ nguyên", `{"title":"Khoá A"}`, false, ""},
		{"null = xoá", `{"discount_price":null}`, true, ""},
		{"chuỗi rỗng = xoá", `{"discount_price":""}`, true, ""},
		{"số = đặt giá mới", `{"discount_price":199000}`, false, "199000"},
		{"chuỗi số = đặt giá mới", `{"discount_price":"199000"}`, false, "199000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req UpdateCourseDTO
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if req.DiscountPriceCleared != tc.wantCleared {
				t.Fatalf("DiscountPriceCleared = %v, muốn %v", req.DiscountPriceCleared, tc.wantCleared)
			}
			got := ""
			if req.DiscountPrice != nil {
				got = req.DiscountPrice.String()
			}
			if got != tc.wantPrice {
				t.Fatalf("DiscountPrice = %q, muốn %q", got, tc.wantPrice)
			}
		})
	}
}

func TestUpdateCourseDTO_OtherFieldsStillParsed(t *testing.T) {
	var req UpdateCourseDTO
	if err := json.Unmarshal([]byte(`{"title":"Khoá A","price":500000,"discount_price":null,"is_free":false}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Title == nil || *req.Title != "Khoá A" || req.Price == nil || req.Price.String() != "500000" || req.IsFree == nil {
		t.Fatalf("các trường khác bị mất: %+v", req)
	}
	if err := json.Unmarshal([]byte(`{"discount_price":"abc"}`), &UpdateCourseDTO{}); err == nil {
		t.Fatal("discount_price không phải số phải bị từ chối")
	}
}

func strPtr(s string) *string { return &s }
