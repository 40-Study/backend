package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type WhiteboardHandlerInterface interface {
	GetSnapshot(c *fiber.Ctx) error
	SaveSnapshot(c *fiber.Ctx) error
}

type WhiteboardHandler struct {
	svc        service.WhiteboardServiceInterface
	livekitSvc service.LivekitServiceInterface
}

func NewWhiteboardHandler(svc service.WhiteboardServiceInterface, livekitSvc service.LivekitServiceInterface) *WhiteboardHandler {
	return &WhiteboardHandler{svc: svc, livekitSvc: livekitSvc}
}

func (h *WhiteboardHandler) GetSnapshot(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	snapshot, err := h.svc.GetSnapshot(c.Context(), sessionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": snapshot})
}

func (h *WhiteboardHandler) SaveSnapshot(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	var req dto.WhiteboardSnapshotDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	req.SessionID = sessionID.String()

	if err := h.svc.SaveSnapshot(c.Context(), req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Snapshot saved"})
}

func (h *WhiteboardHandler) BroadcastEvent(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	var event dto.WhiteboardEventDTO
	if err := c.BodyParser(&event); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.svc.BroadcastEvent(c.Context(), sessionID, event, h.livekitSvc); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Event broadcasted"})
}
