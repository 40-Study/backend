package service

import (
	"errors"

	"study.com/v1/internal/model"
)

// ErrCourseStatusChangeNotAllowed — handler ánh xạ sang 400 code COURSE_STATUS_CHANGE_NOT_ALLOWED.
var ErrCourseStatusChangeNotAllowed = errors.New("course status cannot be changed directly — use submit-review / admin approval")

// ValidateManualCourseStatusChange (Phase 3 duyệt khoá học) chặn PUT /courses/:id đổi status tuỳ
// ý. TRƯỚC phase 3 giáo viên gửi thẳng {"status":"published"} để tự xuất bản — không ai duyệt.
// Giờ PUT chỉ còn 2 chuyển đổi thủ công vô hại (không đi vòng qua duyệt):
//   - published -> archived (ngừng bán, như trước)
//   - archived  -> draft    (mở lại để sửa; muốn xuất bản lại phải nộp duyệt)
//
// Giữ nguyên status (target == current) luôn hợp lệ để client gửi lại cả object không bị lỗi.
// Luật áp dụng cho CẢ admin: admin xuất bản bằng POST /admin/courses/:id/approve để có dấu vết
// reviewed_by/reviewed_at.
func ValidateManualCourseStatusChange(current, target string) error {
	if current == target {
		return nil
	}
	if current == model.CourseStatusPublished && target == model.CourseStatusArchived {
		return nil
	}
	if current == model.CourseStatusArchived && target == model.CourseStatusDraft {
		return nil
	}
	return ErrCourseStatusChangeNotAllowed
}
