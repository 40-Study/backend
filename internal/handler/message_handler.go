package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type MessageHandler struct {
	convService service.ConversationServiceInterface
}

func NewMessageHandler(convService service.ConversationServiceInterface) *MessageHandler {
	return &MessageHandler{convService: convService}
}

// ─── Conversation endpoints ─────────────────────────────────────────────────

// ListConversations - GET /api/conversations
func (h *MessageHandler) ListConversations(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	result, err := h.convService.ListConversations(c.Context(), userID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Conversations retrieved successfully",
		"data":    result,
	})
}

// CreateDirectConversation - POST /api/conversations/direct
func (h *MessageHandler) CreateDirectConversation(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	var req dto.CreateDirectConversationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.convService.CreateDirectConversation(c.Context(), userID, req.UserID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Conversation created successfully",
		"data":    result,
	})
}

// GetConversation - GET /api/conversations/:id
func (h *MessageHandler) GetConversation(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	result, err := h.convService.GetConversation(c.Context(), userID, convID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Conversation retrieved successfully",
		"data":    result,
	})
}

// ─── Conversation actions ───────────────────────────────────────────────────

// MarkAsRead - POST /api/conversations/:id/read
func (h *MessageHandler) MarkAsRead(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	if err := h.convService.MarkAsRead(c.Context(), userID, convID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Marked as read",
	})
}

// MuteConversation - POST /api/conversations/:id/mute
func (h *MessageHandler) MuteConversation(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	if err := h.convService.MuteConversation(c.Context(), userID, convID, true); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Conversation muted",
	})
}

// UnmuteConversation - POST /api/conversations/:id/unmute
func (h *MessageHandler) UnmuteConversation(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	if err := h.convService.MuteConversation(c.Context(), userID, convID, false); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Conversation unmuted",
	})
}

// PinConversation - POST /api/conversations/:id/pin
func (h *MessageHandler) PinConversation(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	if err := h.convService.PinConversation(c.Context(), userID, convID, true); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Conversation pinned",
	})
}

// UnpinConversation - POST /api/conversations/:id/unpin
func (h *MessageHandler) UnpinConversation(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	if err := h.convService.PinConversation(c.Context(), userID, convID, false); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Conversation unpinned",
	})
}

// ─── Message endpoints ──────────────────────────────────────────────────────

// ListMessages - GET /api/conversations/:id/messages
func (h *MessageHandler) ListMessages(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	result, err := h.convService.ListMessages(c.Context(), userID, convID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Messages retrieved successfully",
		"data":    result,
	})
}

// SendMessage - POST /api/conversations/:id/messages
func (h *MessageHandler) SendMessage(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	var req dto.SendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	result, err := h.convService.SendMessage(c.Context(), userID, convID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Message sent successfully",
		"data":    result,
	})
}

// EditMessage - PUT /api/conversations/:id/messages/:messageId
func (h *MessageHandler) EditMessage(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	messageID, err := uuid.Parse(c.Params("messageId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid message ID",
		})
	}

	var req dto.EditMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.convService.EditMessage(c.Context(), userID, convID, messageID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Message edited successfully",
		"data":    result,
	})
}

// DeleteMessage - DELETE /api/conversations/:id/messages/:messageId
func (h *MessageHandler) DeleteMessage(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	messageID, err := uuid.Parse(c.Params("messageId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid message ID",
		})
	}

	if err := h.convService.DeleteMessage(c.Context(), userID, convID, messageID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Message deleted successfully",
	})
}

// ─── Message actions ────────────────────────────────────────────────────────

// PinMessage - POST /api/conversations/:id/messages/:messageId/pin
func (h *MessageHandler) PinMessage(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	messageID, err := uuid.Parse(c.Params("messageId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid message ID",
		})
	}

	if err := h.convService.PinMessage(c.Context(), userID, convID, messageID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Message pinned",
	})
}

// UnpinMessage - POST /api/conversations/:id/messages/:messageId/unpin
func (h *MessageHandler) UnpinMessage(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	messageID, err := uuid.Parse(c.Params("messageId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid message ID",
		})
	}

	if err := h.convService.UnpinMessage(c.Context(), userID, convID, messageID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Message unpinned",
	})
}

// ─── Reaction endpoints ─────────────────────────────────────────────────────

// AddReaction - POST /api/conversations/:id/messages/:messageId/reactions
func (h *MessageHandler) AddReaction(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	messageID, err := uuid.Parse(c.Params("messageId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid message ID",
		})
	}

	var req dto.AddReactionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	if err := h.convService.AddReaction(c.Context(), userID, convID, messageID, req.Emoji); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Reaction added",
	})
}

// RemoveReaction - DELETE /api/conversations/:id/messages/:messageId/reactions/:emoji
func (h *MessageHandler) RemoveReaction(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	convID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid conversation ID",
		})
	}

	messageID, err := uuid.Parse(c.Params("messageId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid message ID",
		})
	}

	emoji := c.Params("emoji")

	if err := h.convService.RemoveReaction(c.Context(), userID, convID, messageID, emoji); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Reaction removed",
	})
}

// ─── Search endpoints ───────────────────────────────────────────────────────

// SearchMessages - GET /api/messages/search
func (h *MessageHandler) SearchMessages(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	keyword := c.Query("q")
	if keyword == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Search query is required",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	var convID *uuid.UUID
	if cid := c.Query("conversation_id"); cid != "" {
		parsed, err := uuid.Parse(cid)
		if err == nil {
			convID = &parsed
		}
	}

	result, err := h.convService.SearchMessages(c.Context(), userID, keyword, convID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Search results",
		"data":    result,
	})
}

// ─── Unread count ───────────────────────────────────────────────────────────

// GetUnreadCount - GET /api/messages/unread
func (h *MessageHandler) GetUnreadCount(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	result, err := h.convService.GetUnreadCount(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Unread count retrieved",
		"data":    result,
	})
}

// UploadAttachment - POST /api/attachments/upload
func (h *MessageHandler) UploadAttachment(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"message": "Not implemented - use upload endpoint",
	})
}
