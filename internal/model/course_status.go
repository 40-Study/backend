package model

// Phase 3 duyệt khoá học + duyệt giáo viên (2026-09-28) — NGUỒN SỰ THẬT DUY NHẤT cho giá trị
// hợp lệ của courses.status và teacher_profiles.approval_status, cùng khuôn với OrderStatuses
// (order_status.go):
//   - RunPostMigrations (internal/database/migrations.go) SINH câu SQL CHECK constraint từ các
//     slice này (buildCheckConstraintSQL) — không chép tay danh sách giá trị lần thứ 2.
//   - Tag `check:` của Course.Status / TeacherProfile.ApprovalStatus buộc phải là literal tĩnh
//     (giới hạn cú pháp struct tag) — course_status_test.go đối chiếu tag với slice ở CẢ HAI
//     CHIỀU, sửa một bên mà quên bên kia sẽ đỏ ngay.

const (
	CourseStatusDraft         = "draft"
	CourseStatusPendingReview = "pending_review"
	CourseStatusPublished     = "published"
	CourseStatusRejected      = "rejected"
	CourseStatusArchived      = "archived"
)

// CourseStatuses — thứ tự không mang nghĩa. "rejected" là giá trị MỚI của phase 3: trước đây
// constraint chk_courses_status chỉ có 4 giá trị nên admin không có cách từ chối một khoá học.
var CourseStatuses = []string{
	CourseStatusDraft, CourseStatusPendingReview, CourseStatusPublished, CourseStatusRejected, CourseStatusArchived,
}

const (
	TeacherApprovalPending  = "pending"
	TeacherApprovalApproved = "approved"
	TeacherApprovalRejected = "rejected"
)

var TeacherApprovalStatuses = []string{
	TeacherApprovalPending, TeacherApprovalApproved, TeacherApprovalRejected,
}

// MaxTeacherResubmissions — quyết định #5 của chủ dự án: hồ sơ giáo viên bị từ chối được sửa và
// NỘP LẠI tối đa 3 lần (không tính lần nộp đầu). Lần nộp lại thứ 4 bị chặn, web hiển thị thông
// báo liên hệ hỗ trợ.
const MaxTeacherResubmissions = 3
