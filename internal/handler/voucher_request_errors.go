package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

// QA6-A05 (contract C9): mỗi lỗi 400 của tạo/sửa voucher phải NÓI RÕ trường nào sai và vì sao (tiếng Việt), thay vì một câu
// chung. Gồm: số nguyên (VND và điểm), khoảng giá trị, độ dài tên, và lỗi giải mã JSON (tràn int32, số lẻ vào trường điểm).

// voucherFieldLabels: tên hiển thị của từng trường JSON của voucher trong câu báo lỗi.
var voucherFieldLabels = map[string]string{
	"code": "Mã voucher", "name": "Tên voucher", "discount_unit": "Loại giảm giá", "discount_method": "Cách giảm giá",
	"discount_amount_money": "Số tiền giảm", "discount_amount_points": "Số điểm giảm", "discount_percent": "Phần trăm giảm",
	"max_discount_money": "Mức giảm tối đa", "max_discount_points": "Số điểm giảm tối đa",
	"min_purchase_money": "Đơn tối thiểu", "min_purchase_points": "Số điểm tối thiểu",
	"usage_limit": "Tổng số lượt dùng", "usage_per_user": "Số lượt dùng mỗi người",
}

// voucherMoneyFields: trường tiền VND, chỉ nhận số nguyên (VND không có đơn vị nhỏ hơn đồng) và vừa cột decimal(12,2).
const voucherMaxMoney = 9_999_999_999

func voucherLabel(field string) string {
	if l, ok := voucherFieldLabels[field]; ok {
		return l
	}
	return field
}

// voucherInvalidBody trả lời lỗi giải mã JSON: trường biết được -> ERR_VALIDATION nêu đúng trường; còn lại ERR_INVALID_REQUEST.
func voucherInvalidBody(c *fiber.Ctx, err error) error {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		field := ute.Field
		if i := strings.LastIndex(field, "."); i >= 0 {
			field = field[i+1:]
		}
		if field == "" { // DTO cập nhật giải mã vài trường riêng: lỗi bọc dạng "<field>: json: ..."
			field, _, _ = strings.Cut(err.Error(), ": ")
		}
		if _, known := voucherFieldLabels[field]; known {
			msg := fmt.Sprintf("%s không hợp lệ.", voucherLabel(field))
			switch {
			case strings.HasPrefix(ute.Type.String(), "int"):
				msg = fmt.Sprintf("%s phải là số nguyên từ 0 đến %d, không có phần thập phân.", voucherLabel(field), math.MaxInt32)
			case strings.HasPrefix(ute.Type.String(), "float"):
				msg = fmt.Sprintf("%s phải là một số.", voucherLabel(field))
			}
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"code": "ERR_VALIDATION", "message": msg,
				"errors": []utils.ValidationError{{Field: field, Tag: "type", Message: msg}},
			})
		}
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "ERR_INVALID_REQUEST", "message": "Dữ liệu gửi lên không hợp lệ. Vui lòng kiểm tra lại các ô đã nhập."})
}

// voucherValidationMessage dịch lỗi của validate tag thành câu tiếng Việt nêu tên trường (lỗi đầu tiên làm `message`).
func voucherValidationMessage(e utils.ValidationError) string {
	l := voucherLabel(e.Field)
	switch {
	case e.Tag == "required":
		return fmt.Sprintf("Vui lòng nhập %s.", strings.ToLower(l))
	case e.Field == "name":
		return "Tên voucher phải có từ 3 đến 255 ký tự."
	case e.Field == "code":
		return "Mã voucher phải có từ 3 đến 50 ký tự."
	case e.Field == "discount_percent":
		return "Phần trăm giảm phải lớn hơn 0 và không quá 100."
	case e.Tag == "oneof":
		return fmt.Sprintf("%s không hợp lệ.", l)
	case e.Tag == "gt":
		return fmt.Sprintf("%s phải lớn hơn 0.", l)
	case e.Tag == "min":
		return fmt.Sprintf("%s không được âm.", l)
	}
	return fmt.Sprintf("%s không hợp lệ.", l)
}

// voucherValidationFailed trả 400 ERR_VALIDATION: `message` nêu trường sai đầu tiên, `errors` liệt kê đủ.
func voucherValidationFailed(c *fiber.Ctx, errs []utils.ValidationError) error {
	out := make([]utils.ValidationError, len(errs))
	for i, e := range errs {
		if e.Tag != "whole" && e.Tag != "maxmoney" { // hai tag này đã mang sẵn câu tiếng Việt đúng trường
			e.Message = voucherValidationMessage(e)
		}
		out[i] = e
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "ERR_VALIDATION", "message": out[0].Message, "errors": out})
}

// voucherWholeMoneyErrors: trường tiền phải là số nguyên đồng và không vượt cột decimal(12,2).
func voucherWholeMoneyErrors(money map[string]*float64) []utils.ValidationError {
	var errs []utils.ValidationError
	for _, field := range []string{"discount_amount_money", "max_discount_money", "min_purchase_money"} {
		v := money[field]
		if v == nil {
			continue
		}
		switch {
		case *v != math.Trunc(*v):
			errs = append(errs, utils.ValidationError{Field: field, Tag: "whole", Message: fmt.Sprintf("%s phải là số nguyên đồng (VND không có phần thập phân).", voucherLabel(field))})
		case *v > voucherMaxMoney:
			errs = append(errs, utils.ValidationError{Field: field, Tag: "maxmoney", Message: fmt.Sprintf("%s không được vượt quá 9.999.999.999 đ.", voucherLabel(field))})
		}
	}
	return errs
}

func voucherCreateMoney(r *dto.CreateVoucherRequest) map[string]*float64 {
	return map[string]*float64{"discount_amount_money": r.DiscountAmountMoney, "max_discount_money": r.MaxDiscountMoney, "min_purchase_money": r.MinPurchaseMoney}
}

func voucherUpdateMoney(r *dto.UpdateVoucherRequest) map[string]*float64 {
	return map[string]*float64{"discount_amount_money": r.DiscountAmountMoney, "max_discount_money": r.MaxDiscountMoney, "min_purchase_money": r.MinPurchaseMoney}
}
