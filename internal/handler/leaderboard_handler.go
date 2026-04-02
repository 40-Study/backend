package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type LeaderboardHandlerInterface interface {
	GetLeaderboard(c *fiber.Ctx) error
	GetMyRank(c *fiber.Ctx) error
}

type LeaderboardHandler struct {
	svc service.LeaderboardServiceInterface
}

func NewLeaderboardHandler(svc service.LeaderboardServiceInterface) *LeaderboardHandler {
	return &LeaderboardHandler{svc: svc}
}

// GetLeaderboard GET /leaderboard?period=weekly|monthly|all_time&limit=100
func (h *LeaderboardHandler) GetLeaderboard(c *fiber.Ctx) error {
	periodType := c.Query("period", "all_time")
	limit := c.QueryInt("limit", 100)

	resp, err := h.svc.GetLeaderboard(c.Context(), periodType, limit)
	if err != nil {
		if err.Error() == "invalid period: must be weekly, monthly, or all_time" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": resp})
}

// GetMyRank GET /leaderboard/me?period=weekly|monthly|all_time
func (h *LeaderboardHandler) GetMyRank(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	periodType := c.Query("period", "all_time")

	resp, err := h.svc.GetMyRank(c.Context(), userID, periodType)
	if err != nil {
		if err.Error() == "invalid period: must be weekly, monthly, or all_time" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": resp})
}
