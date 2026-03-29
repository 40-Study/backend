package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type LessonContentHandler struct {
	service service.LessonContentServiceInterface
}

func NewLessonContentHandler(service service.LessonContentServiceInterface) *LessonContentHandler {
	return &LessonContentHandler{service: service}
}

// Content handlers

func (h *LessonContentHandler) CreateContent(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	var req dto.CreateLessonContentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	content, err := h.service.CreateContent(c.Context(), lessonID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create content", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Content created successfully", "data": content,
	})
}

func (h *LessonContentHandler) GetContent(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	contents, err := h.service.GetContentsByLessonID(c.Context(), lessonID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve contents", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Contents retrieved successfully", "data": contents,
	})
}

func (h *LessonContentHandler) UpdateContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID", "error": err.Error(),
		})
	}

	var req dto.UpdateLessonContentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	content, err := h.service.UpdateContent(c.Context(), contentID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update content", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Content updated successfully", "data": content,
	})
}

func (h *LessonContentHandler) DeleteContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID", "error": err.Error(),
		})
	}

	if err := h.service.DeleteContent(c.Context(), contentID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete content", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Content deleted successfully",
	})
}

// Session handlers

func (h *LessonContentHandler) CreateSession(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	var req dto.CreateLessonSessionDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	session, err := h.service.CreateSession(c.Context(), lessonID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create session", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Session created successfully", "data": session,
	})
}

func (h *LessonContentHandler) GetSessions(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	sessions, err := h.service.GetSessionsByLessonID(c.Context(), lessonID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve sessions", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Sessions retrieved successfully", "data": sessions,
	})
}

func (h *LessonContentHandler) UpdateSession(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID", "error": err.Error(),
		})
	}

	var req dto.UpdateLessonSessionDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	session, err := h.service.UpdateSession(c.Context(), sessionID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update session", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Session updated successfully", "data": session,
	})
}

func (h *LessonContentHandler) DeleteSession(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID", "error": err.Error(),
		})
	}

	if err := h.service.DeleteSession(c.Context(), sessionID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete session", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Session deleted successfully",
	})
}
