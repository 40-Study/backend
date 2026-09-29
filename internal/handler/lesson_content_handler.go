package handler

import (
	"errors"

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

// writeUploadOwnership ánh xạ lỗi kiểm chủ upload khi gắn video vào nội dung bài học:
// 403 UPLOAD_NOT_OWNED (upload của người khác) / 404 UPLOAD_NOT_FOUND (upload không tồn tại).
// Trả true nếu đã ghi response.
func writeUploadOwnership(c *fiber.Ctx, err error) bool {
	switch {
	case errors.Is(err, service.ErrUploadNotOwned):
		_ = c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "Forbidden",
			"code":    "UPLOAD_NOT_OWNED",
			"error":   "Video này không phải do bạn tải lên.",
		})
		return true
	case errors.Is(err, service.ErrUploadNotFound):
		_ = c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Not found",
			"code":    "UPLOAD_NOT_FOUND",
			"error":   "Không tìm thấy video đã tải lên.",
		})
		return true
	}
	return false
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
		if writeCourseLocked(c, err) {
			return nil
		}
		if writeUploadOwnership(c, err) {
			return nil
		}
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

	// Phase 1 §2: userID de service chan noi dung bai bi khoa (ErrLessonLocked -> 403 ben duoi).
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	contents, err := h.service.GetContentsByLessonID(c.Context(), lessonID, userID, isAdmin)
	if err != nil {
		if err == service.ErrLessonLocked {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"message": "LESSON_LOCKED",
			})
		}
		// Quyết định team lead (review vòng 2): lessonID không thuộc khoá đang xét -> 404 rõ
		// ràng, không lẫn với 403 LESSON_LOCKED và không mở lén.
		if err == service.ErrLessonNotInCourse {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Lesson does not belong to this course",
			})
		}
		// D4 (review PR #79): khoá chưa xuất bản mà người gọi không được xem -> 404.
		if err == service.ErrCourseHidden {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Lesson not found"})
		}
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
		if writeCourseLocked(c, err) {
			return nil
		}
		if writeUploadOwnership(c, err) {
			return nil
		}
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
		if writeCourseLocked(c, err) {
			return nil
		}
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
		if writeCourseLocked(c, err) {
			return nil
		}
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
