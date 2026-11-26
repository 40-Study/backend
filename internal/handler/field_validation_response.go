package handler

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/utils"
)

// QA7 (C12, C13): lỗi nhập liệu của một trường luôn là 400 ERR_VALIDATION, `message` tiếng Việt nêu đúng trường và quy tắc,
// `errors` liệt kê từng trường (cùng dạng voucherValidationFailed), không bao giờ chứa err.Error() của Go/DB.

// internalErrorMessage: câu chung cho 500 (kèm code ERR_INTERNAL); chi tiết chỉ ghi vào log server.
const internalErrorMessage = "Đã có lỗi xảy ra, vui lòng thử lại"

const (
	lessonDurationMessage = "Thời lượng bài học phải là số phút nguyên từ 0 đến 1440."
	coursePriceMax        = 1_000_000_000
	lessonDurationMaxMins = 1440
)

func fieldValidationFailed(c *fiber.Ctx, errs ...utils.ValidationError) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "ERR_VALIDATION", "message": errs[0].Message, "errors": errs})
}

// bodyFieldError: lỗi giải mã JSON do kiểu của MỘT trường (vd duration_minutes = 2.5) -> ERR_VALIDATION nêu trường đó;
// ok=false khi không xác định được trường (JSON hỏng, ...) để handler dùng câu chung.
func bodyFieldError(err error, messages map[string]string) (utils.ValidationError, bool) {
	var ute *json.UnmarshalTypeError
	if !errors.As(err, &ute) {
		return utils.ValidationError{}, false
	}
	field := ute.Field
	if i := strings.LastIndex(field, "."); i >= 0 {
		field = field[i+1:]
	}
	msg, ok := messages[field]
	if !ok {
		return utils.ValidationError{}, false
	}
	return utils.ValidationError{Field: field, Tag: "type", Message: msg}, true
}

// invalidBodyResponse: 400 cho body không giải mã được. Trường biết trước -> ERR_VALIDATION nêu trường; còn lại
// ERR_INVALID_REQUEST với câu chung (không lộ lỗi giải mã của Go).
func invalidBodyResponse(c *fiber.Ctx, err error, fieldMessages map[string]string) error {
	if fe, ok := bodyFieldError(err, fieldMessages); ok {
		return fieldValidationFailed(c, fe)
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"code": "ERR_INVALID_REQUEST", "message": "Dữ liệu gửi lên không hợp lệ. Vui lòng kiểm tra lại các ô đã nhập.",
	})
}

// lessonDurationError: duration_minutes là số phút nguyên 0..1440 (nil = không gửi, hợp lệ).
func lessonDurationError(mins *int) []utils.ValidationError {
	if mins == nil || (*mins >= 0 && *mins <= lessonDurationMaxMins) {
		return nil
	}
	return []utils.ValidationError{{Field: "duration_minutes", Tag: "range", Message: lessonDurationMessage}}
}

var courseMoneyLabels = map[string]string{"price": "Giá bán", "discount_price": "Giá khuyến mãi"}

// courseMoneyError: tiền khoá học là số nguyên đồng VND trong 0..1.000.000.000 (nil = không gửi, hợp lệ).
func courseMoneyError(field string, v *decimal.Decimal) []utils.ValidationError {
	if v == nil {
		return nil
	}
	label := courseMoneyLabels[field]
	rangeErr := []utils.ValidationError{{Field: field, Tag: "range", Message: label + " phải từ 0 đến 1.000.000.000 đ."}}
	// QA7 review M1: số mũ nằm ngoài biên (1e20000000, 1e-20000000) bị loại TRƯỚC mọi phép so sánh/làm tròn,
	// xem utils.DecimalExponentWithinBounds. Số mũ dương quá lớn là "vượt 1 tỷ"; số mũ âm quá sâu là phần thập phân.
	if v.Exponent() > utils.DecimalExponentMax {
		return rangeErr
	}
	if !utils.DecimalExponentWithinBounds(*v) || !v.IsInteger() {
		return []utils.ValidationError{{Field: field, Tag: "whole", Message: label + " phải là số nguyên đồng (VND không có phần thập phân)."}}
	}
	if v.IsNegative() || v.GreaterThan(decimal.NewFromInt(coursePriceMax)) {
		return rangeErr
	}
	return nil
}
