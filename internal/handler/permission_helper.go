package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/middleware"
)

// isAdminActor (C-12/H-11, audit 260909 vòng 2): CourseHandler/SectionHandler/LessonHandler/
// ClassHandler cần cho phép SYSTEM_ADMIN sửa/xóa tài nguyên của người khác (kiểm duyệt nội
// dung vi phạm, xử lý lớp học có vấn đề) ngoài chủ sở hữu/giáo viên lớp. Dùng lại permission
// "SYSTEM_SETTINGS_MANAGE" — cùng permission đã dùng để gate route admin coin/voucher/upload
// (xem code-reviewer-260909-1340-backend-security-logic.md, C-08/C-09/C-14) để không phải
// thêm permission mới vào seeder.
//
// permChecker có thể nil (constructor không truyền, ví dụ trong test) — coi như không phải
// admin, KHÔNG panic; lỗi resolve permission cũng coi như không phải admin (fail-closed —
// không mở rộng quyền khi không chắc chắn).
func isAdminActor(c *fiber.Ctx, permChecker *middleware.PermissionChecker, userID uuid.UUID) bool {
	if permChecker == nil {
		return false
	}
	var activeOrgID *uuid.UUID
	if orgID, ok := c.Locals("active_org_id").(uuid.UUID); ok {
		activeOrgID = &orgID
	}
	isAdmin, err := permChecker.HasPermission(c.Context(), userID, activeOrgID, "SYSTEM_SETTINGS_MANAGE")
	if err != nil {
		return false
	}
	return isAdmin
}
