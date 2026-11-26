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

// lessonBodyFieldMessages: duration_minutes có kiểu sai (2.5, "abc") -> ERR_VALIDATION nêu đúng trường (C12).
var lessonBodyFieldMessages = map[string]string{"duration_minutes": lessonDurationMessage}

// lessonValidationFailed: 400 ERR_VALIDATION; thời lượng có câu tiếng Việt riêng, trường khác giữ câu validate chung.
func lessonValidationFailed(c *fiber.Ctx, errs []utils.ValidationError) error {
	msg := "Dữ liệu bài học không hợp lệ."
	for _, e := range errs {
		if e.Field == "duration_minutes" {
			msg = e.Message
			break
		}
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "ERR_VALIDATION", "message": msg, "errors": errs})
}

type LessonHandler struct {
	service     service.LessonServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewLessonHandler(service service.LessonServiceInterface, permChecker *middleware.PermissionChecker) *LessonHandler {
	return &LessonHandler{service: service, permChecker: permChecker}
}

// lessonForbiddenResponse ánh xạ ErrNotLessonCourseOwner (C-12) sang HTTP 403.
func lessonForbiddenResponse(c *fiber.Ctx, err error) bool {
	if writeCourseLocked(c, err) {
		return true
	}
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
		return invalidBodyResponse(c, err, lessonBodyFieldMessages)
	}

	if errs := append(utils.ValidateStruct(req), lessonDurationError(req.DurationMins)...); len(errs) > 0 {
		return lessonValidationFailed(c, errs)
	}

	lesson, err := h.service.CreateLesson(c.Context(), sectionID, userID, req)
	if err != nil {
		if lessonForbiddenResponse(c, err) {
			return nil
		}
		return RespondServiceError(c, err, internalErrorMessage)
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

	// C-1 (review vòng 2): route nằm sau middleware.AuthMiddleware (course_router.go) nên user_id
	// luôn có mặt trên đường thật — cần để service tính locked/lock_reason/progress đúng người
	// đang xem, thay vì trả contents (video_url) cho bất kỳ ai đã đăng nhập.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	lessons, err := h.service.GetAllLessons(c.Context(), sectionID, userID, isAdmin)
	if err != nil {
		// D4 (QA vòng 2): chương không tồn tại / khoá chưa xuất bản mà người xem không có quyền.
		if errors.Is(err, service.ErrCourseHidden) || err.Error() == "section not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Section not found"})
		}
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
		return invalidBodyResponse(c, err, lessonBodyFieldMessages)
	}

	if errs := append(utils.ValidateStruct(req), lessonDurationError(req.DurationMins)...); len(errs) > 0 {
		return lessonValidationFailed(c, errs)
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	lesson, err := h.service.UpdateLesson(c.Context(), lessonID, userID, isAdmin, req)
	if err != nil {
		if lessonForbiddenResponse(c, err) {
			return nil
		}
		// Review S1 M1: file phụ đề (subtitle_url) không do người gọi tải lên -> 403 UPLOAD_NOT_OWNED.
		if writeUploadOwnership(c, err) {
			return nil
		}
		// Phase 1 §4 (bổ sung từ review web #17): message CỐ ĐỊNH, không phải câu tự do —
		// web so khớp đúng chuỗi này để hiện thông báo "bài chưa có video".
		if err == service.ErrLessonHasNoVideo {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "LESSON_HAS_NO_VIDEO",
			})
		}
		return RespondServiceError(c, err, internalErrorMessage)
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
		return RespondServiceError(c, err, internalErrorMessage)
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

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	if err := h.service.ReorderLessons(c.Context(), sectionID, userID, isAdmin, req); err != nil {
		if lessonForbiddenResponse(c, err) {
			return nil
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to reorder lessons",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Lessons reordered successfully",
	})
}
