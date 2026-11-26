package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/repository"
)

// QA7-A09: mọi câu báo lỗi của voucher handler là tiếng Việt, cùng dạng {code,message} với đơn hàng/người dùng:
// ID sai -> 400 ERR_INVALID_ID, không có -> 404 ERR_NOT_FOUND. Lỗi không xác định (DB, ...) KHÔNG đưa err.Error() ra body.

const (
	voucherInvalidIDMessage = "ID voucher không hợp lệ."
	voucherNotFoundMessage  = "Không tìm thấy voucher."
)

func voucherInvalidID(c *fiber.Ctx) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "ERR_INVALID_ID", "message": voucherInvalidIDMessage})
}

func voucherNotFound(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"code": "ERR_NOT_FOUND", "message": voucherNotFoundMessage})
}

// voucherActionFailed: voucher không tồn tại -> 404 ERR_NOT_FOUND; còn lại log phía server và trả `status` + `code` của
// thao tác kèm câu chung `message` (không lộ err.Error() của tầng service/DB).
func voucherActionFailed(c *fiber.Ctx, err error, status int, code, message string) error {
	if errors.Is(err, repository.ErrVoucherNotFound) {
		return voucherNotFound(c)
	}
	// Lỗi nghiệp vụ đã biết (vd "Ngày bắt đầu phải trước ngày kết thúc."): câu tiếng Việt của service an toàn để hiện nguyên văn.
	var known *apperr.KnownError
	if errors.As(err, &known) {
		return c.Status(known.Status).JSON(fiber.Map{"code": code, "message": known.Message})
	}
	log.Printf("[ERROR] %s %s: %v", c.Method(), c.Path(), err)
	return c.Status(status).JSON(fiber.Map{"code": code, "message": message})
}
