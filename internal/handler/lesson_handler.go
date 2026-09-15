package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type LessonHandler struct {
	service     service.LessonServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewLessonHandler(service service.LessonServiceInterface, permChecker *middleware.PermissionChecker) *LessonHandler {
	return &LessonHandler{service: service, permChecker: permChecker}
}

// lessonForbiddenResponse ánh xạ ErrNotLessonCourseOwner (C-12) sang HTTP 403.
func lessonForbiddenResponse(c *fiber.Ctx, err error) bool {
	if err == service.ErrNotLessonCourseOwner {
		_ = c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "You are not the instructor of this course",
		})
		return true
	}
	return false
}

func (h *LessonHandler) CreateLesson(c *fiber.Ctx) error {
	sectionID, err := uuid.Parse(c.Params("section_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.CreateLessonDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	lesson, err := h.service.CreateLesson(c.Context(), sectionID, userID, req)
	if err != nil {
		if lessonForbiddenResponse(c, err) {
			return nil
		}
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
	sectionID, err := uuid.Parse(c.Params("section_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	lessons, err := h.service.GetAllLessons(c.Context(), sectionID)
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

func (h *LessonHandler) GetLessonByID(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID",
			"error":   err.Error(),
		})
	}

	// B-2 (review vòng 2): route nằm sau middleware.AuthMiddleware (course_router.go) nên
	// user_id luôn có mặt trên đường thật — cần để service tính locked/lock_reason đúng người
	// đang xem (xem LessonServiceInterface.GetLessonByID).
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	lesson, err := h.service.GetLessonByID(c.Context(), lessonID, userID, isAdmin)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Lesson not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Lesson retrieved successfully",
		"data":    lesson,
	})
}

func (h *LessonHandler) UpdateLesson(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.UpdateLessonDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	lesson, err := h.service.UpdateLesson(c.Context(), lessonID, userID, isAdmin, req)
	if err != nil {
		if lessonForbiddenResponse(c, err) {
			return nil
		}
		// Phase 1 §4 (bổ sung từ review web #17): message CỐ ĐỊNH, không phải câu tự do —
		// web so khớp đúng chuỗi này để hiện thông báo "bài chưa có video".
		if err == service.ErrLessonHasNoVideo {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "LESSON_HAS_NO_VIDEO",
			})
		}
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
	lessonID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	if err := h.service.DeleteLesson(c.Context(), lessonID, userID, isAdmin); err != nil {
		if lessonForbiddenResponse(c, err) {
			return nil
		}
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
	sectionID, err := uuid.Parse(c.Params("section_id"))
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

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	if err := h.service.ReorderLessons(c.Context(), sectionID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to reorder lessons",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Lessons reordered successfully",
	})
}
