package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/apperr"
)

// RespondServiceError (review vòng 2, MAJOR — PR #69, mục C): trước đây nhiều handler (cart,
// review) trả nguyên văn err.Error() của tầng service/repository ra JSON response — kể cả lỗi hạ
// tầng không xác định (DB, GORM, ...), có thể lộ chi tiết nội bộ cho client.
//
// Chỉ trả nguyên văn message khi err là *apperr.KnownError (lỗi nghiệp vụ đã biết: validate/
// không tìm thấy/đã tồn tại/không có quyền) — dùng đúng Status của nó. Mọi err khác (không xác
// định) bị chặn lại: log phía server, trả 500 với genericMessage chung, KHÔNG bao giờ đưa
// err.Error() của nó vào response.
func RespondServiceError(c *fiber.Ctx, err error, genericMessage string) error {
	var known *apperr.KnownError
	if errors.As(err, &known) {
		body := fiber.Map{"message": known.Message}
		if known.Code != "" {
			body["code"] = known.Code
		}
		return c.Status(known.Status).JSON(body)
	}

	log.Printf("[ERROR] %s %s: %v", c.Method(), c.Path(), err)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"code":    "ERR_INTERNAL",
		"message": genericMessage,
	})
}

// invalidIDResponse: 400 {code: ERR_INVALID_ID, message} cho id trên URL/query sai định dạng; message tiếng Việt nêu đối tượng,
// không bao giờ chứa lỗi parse của Go (vd "invalid UUID length: 14").
func invalidIDResponse(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "ERR_INVALID_ID", "message": message})
}

// notFoundResponse: 404 {code: ERR_NOT_FOUND, message} (cùng mã apperr.NotFound mang theo).
func notFoundResponse(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"code": apperr.NotFoundCode, "message": message})
}
