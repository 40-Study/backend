package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// AuditLogHandler: đọc nhật ký hoạt động quản trị (contract C2). Ghi nhật ký KHÔNG đi qua handler
// này mà qua middleware.Audit gắn ở từng route quản trị.
type AuditLogHandler struct {
	svc service.AuditLogServiceInterface
}

func NewAuditLogHandler(svc service.AuditLogServiceInterface) *AuditLogHandler {
	return &AuditLogHandler{svc: svc}
}

// List GET /api/admin/audit-logs
func (h *AuditLogHandler) List(c *fiber.Ctx) error {
	var q dto.AuditLogFilterDTO
	if err := c.QueryParser(&q); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Bộ lọc nhật ký không hợp lệ", "code": "INVALID_FILTER",
		})
	}
	result, err := h.svc.List(c.UserContext(), q)
	if err != nil {
		var bad *service.InvalidAuditFilterError
		if errors.As(err, &bad) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Bộ lọc nhật ký không hợp lệ: " + bad.Field, "code": "INVALID_FILTER",
			})
		}
		return RespondServiceError(c, err, "Không tải được nhật ký hoạt động")
	}
	return c.JSON(fiber.Map{"message": "success", "data": result})
}

// Actions GET /api/admin/audit-logs/actions — SSOT model.AuditActions cho ô lọc ở web.
func (h *AuditLogHandler) Actions(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "success", "data": h.svc.Actions()})
}
