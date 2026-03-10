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
}

type ChatHandler struct {
	svc service.ChatServiceInterface
}

func NewChatHandler(svc service.ChatServiceInterface) *ChatHandler {
	return &ChatHandler{svc: svc}
}

func (h *ChatHandler) Send(c *fiber.Ctx) error {
	var req dto.SendChatMessageDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	message, err := h.svc.SendMessage(c.Context(), req)
	if err != nil {
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

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 50)

	result, err := h.svc.GetMessages(c.Context(), sessionID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *ChatHandler) DeleteMessage(c *fiber.Ctx) error {
	var req dto.DeleteChatMessageDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	messageID, err := uuid.Parse(req.MessageID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid message_id"})
	}

	deletedBy, err := uuid.Parse(req.DeletedBy)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid deleted_by"})
	}

	if err := h.svc.DeleteMessage(c.Context(), messageID, deletedBy); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Message deleted"})
}

func (h *ChatHandler) PinMessage(c *fiber.Ctx) error {
	var req dto.PinChatMessageDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	messageID, err := uuid.Parse(req.MessageID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid message_id"})
	}

	if err := h.svc.PinMessage(c.Context(), messageID, req.IsPinned); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Message pin status updated"})
}
