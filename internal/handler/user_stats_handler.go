package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

type UserStatsHandlerInterface interface {
	GetPublicProfile(c *fiber.Ctx) error
}

type UserStatsHandler struct {
	svc         service.UserStatsServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewUserStatsHandler(svc service.UserStatsServiceInterface, permChecker *middleware.PermissionChecker) *UserStatsHandler {
	return &UserStatsHandler{svc: svc, permChecker: permChecker}
}

// GetPublicProfile GET /users/:id/public-profile
func (h *UserStatsHandler) GetPublicProfile(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	// S6: route công khai nhưng theo người xem (OptionalAuth): khách không có user_id; admin
	// (SYSTEM_SETTINGS_MANAGE) và chính chủ luôn xem đầy đủ.
	var viewerID *uuid.UUID
	viewerIsAdmin := false
	if uid, ok := c.Locals("user_id").(uuid.UUID); ok && uid != uuid.Nil {
		viewerID = &uid
		viewerIsAdmin = isAdminActor(c, h.permChecker, uid)
	}

	profile, err := h.svc.GetPublicProfile(c.Context(), userID, viewerID, viewerIsAdmin)
	if err != nil {
		if errors.Is(err, service.ErrPublicProfileNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": profile})
}
