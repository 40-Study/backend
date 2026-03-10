package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type AnalyticsHandlerInterface interface {
	GetLivestreamAnalytics(c *fiber.Ctx) error
	GetAssignmentAnalytics(c *fiber.Ctx) error
}

type AnalyticsHandler struct {
	svc service.AnalyticsServiceInterface
}

func NewAnalyticsHandler(svc service.AnalyticsServiceInterface) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc}
}

func (h *AnalyticsHandler) GetLivestreamAnalytics(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	analytics, err := h.svc.GetLivestreamAnalytics(c.Context(), sessionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": analytics})
}

func (h *AnalyticsHandler) GetAssignmentAnalytics(c *fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("assignmentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid assignment_id"})
	}

	analytics, err := h.svc.GetAssignmentAnalytics(c.Context(), assignmentID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": analytics})
}

func (h *AnalyticsHandler) GetParticipantAnalytics(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	analytics, err := h.svc.GetParticipantAnalytics(c.Context(), sessionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": analytics})
}
