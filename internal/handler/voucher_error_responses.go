package handler

import (
	"errors"

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

func voucherInvalidID(c *fiber.Ctx) error { return invalidIDResponse(c, voucherInvalidIDMessage) }

func voucherNotFound(c *fiber.Ctx) error { return notFoundResponse(c, voucherNotFoundMessage) }

// voucherLookupFailed: tra voucher theo id/mã. CHỈ "không có" mới là 404 ERR_NOT_FOUND; mọi lỗi khác (DB, ...) // và trả 500 ERR_INTERNAL, không giả làm 404 (QA7 review L6) để web biết thử lại thay vì báo "không có voucher".
func voucherLookupFailed(c *fiber.Ctx, err error) error {
	if errors.Is(err, repository.ErrVoucherNotFound) {
		return voucherNotFound(c)
	}
	return RespondServiceError(c, err, internalErrorMessage)
}

// voucherActionFailed: voucher không tồn tại -> 404 ERR_NOT_FOUND. Lỗi nghiệp vụ đã biết (apperr.KnownError) giữ status, câu
// tiếng Việt và `code` riêng của nó (chỉ khi nó không có code mới dùng `actionCode` của thao tác). Mọi lỗi còn lại (DB, ...) log
// phía server và trả 500 ERR_INTERNAL kèm câu chung `message` của thao tác, không lộ err.Error() (QA7 review M5).
func voucherActionFailed(c *fiber.Ctx, err error, actionCode, message string) error {
	if errors.Is(err, repository.ErrVoucherNotFound) {
		return voucherNotFound(c)
	}
	var known *apperr.KnownError
	if errors.As(err, &known) {
		code := known.Code
		if code == "" {
			code = actionCode
		}
		return c.Status(known.Status).JSON(fiber.Map{"code": code, "message": known.Message})
	}
	return RespondServiceError(c, err, message)
}
