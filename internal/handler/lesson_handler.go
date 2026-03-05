package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type LessonHandler struct {
	service service.LessonServiceInterface
}

func NewLessonHandler(service service.LessonServiceInterface) *LessonHandler {
	return &LessonHandler{service: service}
}

func (h *LessonHandler) CreateLesson(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	sectionID, err := uuid.Parse(c.Params("sectionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	var req dto.CreateLessonDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	lesson, err := h.service.CreateLesson(c.Context(), courseID, sectionID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create lesson",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Lesson created successfully",
		"data":    lesson,
	})
}

func (h *LessonHandler) GetAllLessons(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	sectionID, err := uuid.Parse(c.Params("sectionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	lessons, err := h.service.GetAllLessons(c.Context(), courseID, sectionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve lessons",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Lessons retrieved successfully",
		"data":    lessons,
	})
}

func (h *LessonHandler) UpdateLesson(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	sectionID, err := uuid.Parse(c.Params("sectionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	lessonID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateLessonDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	lesson, err := h.service.UpdateLesson(c.Context(), courseID, sectionID, lessonID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update lesson",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Lesson updated successfully",
		"data":    lesson,
	})
}

func (h *LessonHandler) DeleteLesson(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	sectionID, err := uuid.Parse(c.Params("sectionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	lessonID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.DeleteLesson(c.Context(), courseID, sectionID, lessonID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete lesson",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Lesson deleted successfully",
	})
}

func (h *LessonHandler) ReorderLessons(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	sectionID, err := uuid.Parse(c.Params("sectionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	var req dto.ReorderDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if err := h.service.ReorderLessons(c.Context(), courseID, sectionID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to reorder lessons",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Lessons reordered successfully",
	})
}
