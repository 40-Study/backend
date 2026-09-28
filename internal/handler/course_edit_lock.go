package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/service"
)

// CourseLockedCode — mã lỗi cho client khi sửa khoá đang chờ duyệt (Q5, QA vòng 2). Web đọc mã
// này để hiện hướng dẫn "Rút yêu cầu duyệt" thay vì thông điệp tiếng Anh của backend.
const CourseLockedCode = "COURSE_PENDING_REVIEW"

// writeCourseLocked ghi 409 nếu err là service.ErrCourseLockedForReview; trả false để handler đi
// tiếp các nhánh lỗi cũ. 409 (xung đột trạng thái) chứ không phải 403: người gọi CÓ quyền sửa
// khoá, chỉ là khoá đang ở trạng thái không cho sửa.
func writeCourseLocked(c *fiber.Ctx, err error) bool {
	if !errors.Is(err, service.ErrCourseLockedForReview) {
		return false
	}
	_ = c.Status(fiber.StatusConflict).JSON(fiber.Map{
		"message": "Course is pending review. Withdraw the review request before editing.",
		"code":    CourseLockedCode,
	})
	return true
}
