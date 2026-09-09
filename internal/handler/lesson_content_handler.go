package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type LessonContentHandler struct {
	service     service.LessonContentServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewLessonContentHandler(service service.LessonContentServiceInterface, permChecker *middleware.PermissionChecker) *LessonContentHandler {
	return &LessonContentHandler{service: service, permChecker: permChecker}
}

// lessonContentErrorStatus (H-05, review vòng 1): ánh xạ ErrNotLessonCourseOwner (dùng chung
// với Lesson, xem lesson_handler.go) sang 403.
func lessonContentErrorStatus(err error) int {
	if err == service.ErrNotLessonCourseOwner {
		return fiber.StatusForbidden
	}
	return 0
}

func (h *LessonContentHandler) CreateContent(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lesson_id"))
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

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	content, err := h.service.CreateContent(c.Context(), lessonID, userID, isAdmin, req)
	if err != nil {
		if status := lessonContentErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create content", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Content created successfully", "data": content,
	})
}

func (h *LessonContentHandler) GetContent(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lesson_id"))
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

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	content, err := h.service.UpdateContent(c.Context(), contentID, userID, isAdmin, req)
	if err != nil {
		if status := lessonContentErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update content", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Content updated successfully", "data": content,
	})
}

func (h *LessonContentHandler) ReorderContents(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lesson_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	var req dto.ReorderDTO
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

	// M2-03 (review vòng 3): trước đây route này không đọc user_id gì cả — thêm actorUserID/
	// isAdmin để service kiểm chủ sở hữu, khớp pattern UpdateContent/DeleteContent phía trên.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	if err := h.service.ReorderContents(c.Context(), lessonID, userID, isAdmin, req); err != nil {
		if status := lessonContentErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to reorder contents", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Contents reordered successfully",
	})
}

func (h *LessonContentHandler) DeleteContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID", "error": err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	if err := h.service.DeleteContent(c.Context(), contentID, userID, isAdmin); err != nil {
		if status := lessonContentErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": "Forbidden", "error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete content", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Content deleted successfully",
	})
}
