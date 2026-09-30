package dto

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

// Phân biệt "không gửi" với "gửi null / chuỗi rỗng" cho các trường tuỳ chọn có thể XOÁ.
//
// Con trỏ `*T` không làm được việc này: JSON `null` và trường vắng mặt đều ra `nil`, nên trước đây
// người dùng không có cách nào xoá số điện thoại (hồ sơ) hay giá khuyến mãi (khoá học) — request
// "xoá" bị bỏ qua và web phải báo lỗi thay vì lưu. Quy ước chung của hai DTO dưới đây:
//
//	trường vắng mặt          => giữ nguyên giá trị hiện tại
//	trường = null hoặc ""    => XOÁ (đặt NULL)
//	trường có giá trị        => đặt giá trị mới (vẫn qua validate như cũ)
//
// Cờ *Cleared là `json:"-"`: chỉ do UnmarshalJSON đặt, client không gửi trực tiếp được.

// isJSONNullOrEmptyString cho biết raw là `null` hoặc chuỗi rỗng `""`.
func isJSONNullOrEmptyString(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte(`""`))
}

// UnmarshalJSON của UpdateMeRequestDto đọc `phone` theo quy ước ở trên.
func (r *UpdateMeRequestDto) UnmarshalJSON(data []byte) error {
	// `plain` không có method UnmarshalJSON nên gọi lại json.Unmarshal không đệ quy vô hạn.
	type plain UpdateMeRequestDto
	aux := struct {
		*plain
		// Field cùng tên JSON nằm nông hơn nên thắng field `Phone` của plain: nhận thô để thấy `null`.
		Phone json.RawMessage `json:"phone"`
	}{plain: (*plain)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	r.Phone, r.PhoneCleared = nil, false
	if len(aux.Phone) == 0 {
		return nil // vắng mặt: giữ nguyên
	}
	if isJSONNullOrEmptyString(aux.Phone) {
		r.PhoneCleared = true
		return nil
	}
	var phone string
	if err := json.Unmarshal(aux.Phone, &phone); err != nil {
		return fmt.Errorf("phone: %w", err)
	}
	r.Phone = &phone
	return nil
}

// UnmarshalJSON của UpdateCourseDTO đọc `discount_price` theo quy ước ở trên.
func (r *UpdateCourseDTO) UnmarshalJSON(data []byte) error {
	type plain UpdateCourseDTO
	aux := struct {
		*plain
		DiscountPrice json.RawMessage `json:"discount_price"`
	}{plain: (*plain)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	r.DiscountPrice, r.DiscountPriceCleared = nil, false
	if len(aux.DiscountPrice) == 0 {
		return nil
	}
	if isJSONNullOrEmptyString(aux.DiscountPrice) {
		r.DiscountPriceCleared = true
		return nil
	}
	var price decimal.Decimal
	if err := json.Unmarshal(aux.DiscountPrice, &price); err != nil {
		return fmt.Errorf("discount_price: %w", err)
	}
	r.DiscountPrice = &price
	return nil
}
