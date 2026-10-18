package handler

import (
	"context"
	"errors"
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type OpenDesignHandler struct{ service *service.OpenDesignService }

func NewOpenDesignHandler(s *service.OpenDesignService) *OpenDesignHandler {
	return &OpenDesignHandler{service: s}
}

func (h *OpenDesignHandler) Status(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"data": fiber.Map{"configured": h.service.Configured(), "provider": "Viettel AI"}})
}

func (h *OpenDesignHandler) Document(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil || file.Size > service.MaxTeachingFileBytes {
		return c.Status(400).JSON(fiber.Map{"message": service.ErrTeachingFile.Error()})
	}
	stream, err := file.Open()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"message": service.ErrTeachingFile.Error()})
	}
	defer stream.Close()
	raw, err := io.ReadAll(io.LimitReader(stream, service.MaxTeachingFileBytes+1))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"message": service.ErrTeachingFile.Error()})
	}
	document, err := service.ExtractTeachingDocument(file.Filename, raw)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"message": err.Error()})
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"data": document})
}

func (h *OpenDesignHandler) Generate(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	if len(c.Body()) > 450000 {
		return c.Status(413).JSON(fiber.Map{"message": "Yêu cầu quá lớn."})
	}
	var input service.DesignRequest
	if c.BodyParser(&input) != nil {
		return c.Status(400).JSON(fiber.Map{"message": "Yêu cầu không hợp lệ."})
	}
	if _, err := uuid.Parse(input.RequestID); err != nil {
		return c.Status(400).JSON(fiber.Map{"message": "request_id không hợp lệ."})
	}
	result, err := h.service.Generate(c.UserContext(), userID.String(), input)
	if err != nil {
		status := fiber.StatusBadGateway
		switch {
		case errors.Is(err, service.ErrDesignInput):
			status = 400
		case errors.Is(err, service.ErrDesignNotConfigured):
			status = 503
		case errors.Is(err, service.ErrDesignBusy):
			status = 409
		case errors.Is(err, service.ErrDesignRateLimit):
			status = 429
		case errors.Is(err, context.Canceled):
			status = 409
			err = errors.New("Đã huỷ yêu cầu AI.")
		case errors.Is(err, context.DeadlineExceeded):
			status = 504
			err = errors.New("Viettel AI phản hồi quá lâu. Hãy thử lại.")
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"data": result})
}

func (h *OpenDesignHandler) Cancel(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.SendStatus(401)
	}
	var input struct {
		RequestID string `json:"request_id"`
	}
	if c.BodyParser(&input) != nil {
		return c.SendStatus(400)
	}
	if _, err := uuid.Parse(input.RequestID); err != nil {
		return c.SendStatus(400)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"cancelled": h.service.Cancel(userID.String(), input.RequestID)}})
}
