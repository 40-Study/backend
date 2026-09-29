package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

// LessonPreviewHandler (F1, QA vòng 2 260929): endpoint CÔNG KHAI — không qua middleware `auth`
// (đăng ký ở course_router.go, nhóm "Courses - public read") — cho khách CHƯA đăng nhập xem nội
// dung (video) của bài học đã đánh dấu "Xem thử" (`is_preview=true`) trên khoá ĐÃ published.
// Trước đây web gọi thẳng `GET /lessons/:lesson_id/contents` (LessonContentHandler.GetContent),
// route đó luôn đứng sau `auth` nên khách luôn nhận 401 — xem course_syllabus.tsx gọi
// useLessonContents. Handler này KHÔNG đọc `c.Locals("user_id")` vì không có (khách ẩn danh) và
// uỷ toàn bộ điều kiện published+preview cho LessonContentService.GetPreviewContentsByLessonID.
type LessonPreviewHandler struct {
	service *service.LessonContentService
}

func NewLessonPreviewHandler(service *service.LessonContentService) *LessonPreviewHandler {
	return &LessonPreviewHandler{service: service}
}

// GetPreviewContents — GET /courses/:slug/preview-lessons/:lesson_id/contents.
func (h *LessonPreviewHandler) GetPreviewContents(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lesson_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}
	slug := c.Params("slug")

	contents, err := h.service.GetPreviewContentsByLessonID(c.Context(), slug, lessonID)
	if err != nil {
		// Cả "khoá không tồn tại/chưa published" lẫn "bài không phải preview" đều trả 404 giống
		// nhau — không cho khách dò ra sự khác biệt giữa hai lý do (F1: "không để lộ khoá nháp").
		if err == service.ErrCourseHidden || err == service.ErrLessonNotPreview || err == service.ErrLessonNotInCourse {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Lesson not found"})
		}
		// M1 (review): route công khai, KHÔNG trả err.Error() (có thể lộ chi tiết DB/nội bộ cho khách).
		// Log để điều tra, trả message chung.
		log.Printf("[ERROR] lesson preview contents (slug=%s lesson=%s): %v", slug, lessonID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve contents",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Contents retrieved successfully", "data": contents,
	})
}
