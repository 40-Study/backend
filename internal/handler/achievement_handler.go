package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type AchievementHandlerInterface interface {
	ListAchievements(c *fiber.Ctx) error
	GetMyAchievements(c *fiber.Ctx) error
	UnlockAchievement(c *fiber.Ctx) error
}

type AchievementHandler struct {
	svc service.AchievementServiceInterface
}

func NewAchievementHandler(svc service.AchievementServiceInterface) *AchievementHandler {
	return &AchievementHandler{svc: svc}
}

// ListAchievements GET /achievements
func (h *AchievementHandler) ListAchievements(c *fiber.Ctx) error {
	achievements, err := h.svc.ListAchievements(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": achievements})
}

// GetMyAchievements GET /achievements/me
func (h *AchievementHandler) GetMyAchievements(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	achievements, err := h.svc.GetMyAchievements(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": achievements})
}

// UnlockAchievement POST /achievements/:id/unlock  (internal use)
func (h *AchievementHandler) UnlockAchievement(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	achievementID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid achievement id"})
	}

	resp, err := h.svc.UnlockAchievement(c.Context(), userID, achievementID)
	if err != nil {
		if err.Error() == "achievement not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		if err.Error() == "achievement already unlocked" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": resp})
}
