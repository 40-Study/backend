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

// RequestChangeEmail handles POST /auth/change-email/request
// Verifies password and sends OTP to the new email address.
func (h *AuthHandler) RequestChangeEmail(c *fiber.Ctx) error {
	userID, ok := getUserIDFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	var req dto.RequestChangeEmailDto
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

	if err := h.authService.RequestChangeEmail(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "OTP sent to new email address",
	})
}

// ChangeEmail handles POST /auth/change-email
// Confirms the email change using the OTP sent to the new address.
func (h *AuthHandler) ChangeEmail(c *fiber.Ctx) error {
	userID, ok := getUserIDFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	var req dto.ChangeEmailDto
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

	if err := h.authService.ChangeEmail(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Email updated successfully",
	})
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

	if err := h.authService.DeleteAccount(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Account deleted successfully",
	})
}

