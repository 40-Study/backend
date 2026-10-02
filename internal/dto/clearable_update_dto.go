package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

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

// decodeClearable đọc một trường tuỳ chọn có thể xoá theo quy ước ở đầu file: vắng mặt → (nil, false);
// null hoặc "" → (nil, true); còn lại giải mã thành *T (lỗi kiểu bị từ chối, không lặng lẽ bỏ qua).
func decodeClearable[T any](raw json.RawMessage, field string) (value *T, cleared bool, err error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	if isJSONNullOrEmptyString(raw) {
		return nil, true, nil
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false, fmt.Errorf("%s: %w", field, err)
	}
	return &v, false, nil
}

// UnmarshalJSON của UpdateVoucherRequest đọc start_date, end_date, max_discount_money và
// max_discount_points theo quy ước ở trên (L6 mục 8): admin xoá được ngày bắt đầu/kết thúc và trần giảm.
func (r *UpdateVoucherRequest) UnmarshalJSON(data []byte) error {
	type plain UpdateVoucherRequest
	aux := struct {
		*plain
		StartDate         json.RawMessage `json:"start_date"`
		EndDate           json.RawMessage `json:"end_date"`
		MaxDiscountMoney  json.RawMessage `json:"max_discount_money"`
		MaxDiscountPoints json.RawMessage `json:"max_discount_points"`
	}{plain: (*plain)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	var err error
	if r.StartDate, r.StartDateCleared, err = decodeClearable[time.Time](aux.StartDate, "start_date"); err != nil {
		return err
	}
	if r.EndDate, r.EndDateCleared, err = decodeClearable[time.Time](aux.EndDate, "end_date"); err != nil {
		return err
	}
	if r.MaxDiscountMoney, r.MaxDiscountMoneyCleared, err = decodeClearable[float64](aux.MaxDiscountMoney, "max_discount_money"); err != nil {
		return err
	}
	if r.MaxDiscountPoints, r.MaxDiscountPointsCleared, err = decodeClearable[int32](aux.MaxDiscountPoints, "max_discount_points"); err != nil {
		return err
	}
	return nil
}
