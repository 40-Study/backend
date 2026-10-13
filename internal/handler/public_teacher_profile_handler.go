package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

// PublicTeacherProfileHandler - PG-M2, contract: plans/261013-qa2-role-findings/contract-be3.md §3.
type PublicTeacherProfileHandler struct {
	service service.PublicTeacherProfileServiceInterface
}

func NewPublicTeacherProfileHandler(service service.PublicTeacherProfileServiceInterface) *PublicTeacherProfileHandler {
	return &PublicTeacherProfileHandler{service: service}
}

// GetPublicProfile GET /teachers/:id/public-profile (không cần đăng nhập).
func (h *PublicTeacherProfileHandler) GetPublicProfile(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid teacher ID", "code": "VALIDATION_FAILED"})
	}
	page, err := h.service.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrTeacherPublicProfileNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Teacher not found"})
		}
		return RespondServiceError(c, err, "Failed to retrieve teacher profile")
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Teacher profile retrieved successfully", "data": page})
}
