package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type UserStatsHandlerInterface interface {
	GetPublicProfile(c *fiber.Ctx) error
}

type UserStatsHandler struct {
	svc service.UserStatsServiceInterface
}

func NewUserStatsHandler(svc service.UserStatsServiceInterface) *UserStatsHandler {
	return &UserStatsHandler{svc: svc}
}

// GetPublicProfile GET /users/:id/public-profile
func (h *UserStatsHandler) GetPublicProfile(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	profile, err := h.svc.GetPublicProfile(c.Context(), userID)
	if err != nil {
		if err.Error() == "user not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": profile})
}
