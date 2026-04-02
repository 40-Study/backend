package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type ClassLessonContentHandler struct {
	service service.ClassLessonContentServiceInterface
}

func NewClassLessonContentHandler(service service.ClassLessonContentServiceInterface) *ClassLessonContentHandler {
	return &ClassLessonContentHandler{service: service}
}

// POST /api/lesson-contents/:id/classes
func (h *ClassLessonContentHandler) AssignClassToContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	userID := c.Locals("user_id").(uuid.UUID)

	var req dto.AssignClassToContentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	result, err := h.service.AssignClassToContent(c.Context(), contentID, userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to assign class to content",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Class assigned to content successfully",
		"data":    result,
	})
}

// PUT /api/lesson-contents/:id/classes/:class_id
func (h *ClassLessonContentHandler) UpdateClassContentSchedule(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	classID, err := uuid.Parse(c.Params("class_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateClassContentScheduleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	result, err := h.service.UpdateClassContentSchedule(c.Context(), contentID, classID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update schedule",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Schedule updated successfully",
		"data":    result,
	})
}

// DELETE /api/lesson-contents/:id/classes/:class_id
func (h *ClassLessonContentHandler) RemoveClassFromContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	classID, err := uuid.Parse(c.Params("class_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemoveClassFromContent(c.Context(), contentID, classID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove class from content",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Class removed from content successfully",
	})
}

// GET /api/lesson-contents/:id/classes
func (h *ClassLessonContentHandler) GetClassesForContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	result, err := h.service.GetClassesForContent(c.Context(), contentID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get classes for content",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Classes retrieved successfully",
		"data":    result,
	})
}

// GET /api/classes/:id/contents
func (h *ClassLessonContentHandler) GetContentScheduleForClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	result, err := h.service.GetContentScheduleForClass(c.Context(), classID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get content schedule for class",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Content schedule retrieved successfully",
		"data":    result,
	})
}

// POST /api/lesson-contents/:id/classes/bulk
func (h *ClassLessonContentHandler) BulkAssignClassesToContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	userID := c.Locals("user_id").(uuid.UUID)

	var req dto.BulkAssignClassesToContentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	result, err := h.service.BulkAssignClassesToContent(c.Context(), contentID, userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to bulk assign classes",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Classes assigned to content successfully",
		"data":    result,
	})
}
