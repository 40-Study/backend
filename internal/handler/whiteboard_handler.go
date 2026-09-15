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

// whiteboardErrorStatus (V3-6, issue #58): phan loai loi UY QUYEN tu WhiteboardService thanh 403,
// cung pattern voi respondForbiddenOrError cua livestream_handler.go.
func whiteboardErrorStatus(err error) int {
	if service.IsForbiddenErr(err) {
		return fiber.StatusForbidden
	}
	return 0
}

func (h *WhiteboardHandler) GetSnapshot(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	// V3-6 (issue #58): chi thanh vien phien moi doc duoc bang trang cua phien — truoc day bat ky
	// user dang nhap nao cung doc duoc.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	snapshot, err := h.svc.GetSnapshot(c.Context(), userID, sessionID)
	if err != nil {
		if status := whiteboardErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": snapshot})
}

func (h *WhiteboardHandler) SaveSnapshot(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	// V3-6 (issue #58): chi thanh vien phien moi duoc ghi de bang trang cua phien — truoc day bat
	// ky user dang nhap nao cung ghi duoc.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	var req dto.WhiteboardSnapshotDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	req.SessionID = sessionID.String()

	if err := h.svc.SaveSnapshot(c.Context(), userID, req); err != nil {
		if status := whiteboardErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Snapshot saved"})
}

func (h *WhiteboardHandler) BroadcastEvent(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	// V3-6 (issue #58): chi thanh vien phien moi duoc phat su kien bang trang vao phien do —
	// truoc day bat ky user dang nhap nao cung phat duoc.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	var event dto.WhiteboardEventDTO
	if err := c.BodyParser(&event); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.svc.BroadcastEvent(c.Context(), userID, sessionID, event, h.livekitSvc); err != nil {
		if status := whiteboardErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Event broadcasted"})
}
