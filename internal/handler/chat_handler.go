package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type ChatHandlerInterface interface {
	Send(c *fiber.Ctx) error
	GetMessages(c *fiber.Ctx) error
	DeleteMessage(c *fiber.Ctx) error
	PinMessage(c *fiber.Ctx) error
	UnPinMessage(c *fiber.Ctx) error
}

type ChatHandler struct {
	svc service.ChatServiceInterface
}

func NewChatHandler(svc service.ChatServiceInterface) *ChatHandler {
	return &ChatHandler{svc: svc}
}

// chatErrorStatus (V3-6, issue #58): phan loai loi UY QUYEN tu ChatService thanh 403 — cung
// pattern voi respondForbiddenOrError cua livestream_handler.go, dung chung service.IsForbiddenErr.
func chatErrorStatus(err error) int {
	if service.IsForbiddenErr(err) {
		return fiber.StatusForbidden
	}
	return 0
}

func (h *ChatHandler) Send(c *fiber.Ctx) error {
	// V3-6 (issue #58): nguoi gui la nguoi goi API THAT SU, lay tu access token. `user_id` cu
	// trong body bi Fiber bo qua lang le (da bi xoa khoi dto.SendChatMessageDTO).
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	var req dto.SendChatMessageDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	message, err := h.svc.SendMessage(c.Context(), userID, req)
	if err != nil {
		if status := chatErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Message sent",
		"data":    message,
	})
}

func (h *ChatHandler) GetMessages(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	// V3-6 (issue #58): chi thanh vien phien moi doc duoc lich su chat — truoc day bat ky user
	// dang nhap nao cung doc duoc chat cua bat ky phien nao.
	userID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 50)

	result, err := h.svc.GetMessages(c.Context(), userID, sessionID, page, pageSize)
	if err != nil {
		if status := chatErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

// DeleteMessage (V3-6, issue #58): message id lay tu URL param `:id` (giong Pin/UnPinMessage),
// nguoi xoa la nguoi goi API that su. Truoc day nhan CA HAI tu body (`message_id`, `deleted_by`)
// — `deleted_by` la truong client tu khai, khong kiem quyen; va trong thuc te web khong bao gio
// gui body cho DELETE nen `uuid.Parse("")` luon loi 400 (xem API_DOCUMENTATION.md).
func (h *ChatHandler) DeleteMessage(c *fiber.Ctx) error {
	messageID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid message_id"})
	}

	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	if err := h.svc.DeleteMessage(c.Context(), actorID, messageID); err != nil {
		if status := chatErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Message deleted"})
}

func (h *ChatHandler) PinMessage(c *fiber.Ctx) error {
	messageID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid message_id"})
	}

	// V3-6 (issue #58): ghim la thao tac KIEM DUYET — chi nguoi quan tri phien. Truoc day khong
	// kiem gi ca.
	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	if err := h.svc.PinMessage(c.Context(), actorID, messageID); err != nil {
		if status := chatErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Message pin status updated"})
}

func (h *ChatHandler) UnPinMessage(c *fiber.Ctx) error {
	messageID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid message_id"})
	}

	actorID, done := callerOrUnauthorized(c)
	if done {
		return nil
	}

	if err := h.svc.UnPinMessage(c.Context(), actorID, messageID); err != nil {
		if status := chatErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Message pin status updated"})
}
