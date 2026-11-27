package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

// getUserIDFromCtx extracts user_id set by AuthMiddleware from Fiber locals.
func getUserIDFromCtx(c *fiber.Ctx) (uuid.UUID, bool) {
	raw := c.Locals("user_id")
	if raw == nil {
		return uuid.Nil, false
	}
	id, ok := raw.(uuid.UUID)
	return id, ok
}

// DeleteAccount handles DELETE /auth/me
// Soft-deletes the authenticated user's account after password verification.
func (h *AuthHandler) DeleteAccount(c *fiber.Ctx) error {
	userID, ok := getUserIDFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	var req dto.DeleteAccountDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	err := h.authService.DeleteAccount(c.Context(), userID, req)
	if handled, sendErr := respondIfKnownError(c, err); handled {
		return sendErr
	}
	if err != nil {
		// Lỗi không xác định (DB, ...) -> 500 ERR_INTERNAL, không đưa err.Error() ra client (#142 M1).
		return RespondServiceError(c, err, "Không thể xoá tài khoản. Vui lòng thử lại sau.")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Account deleted successfully",
	})
}

