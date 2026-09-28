package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupApprovalRoutes — Phase 3 duyệt khoá học + duyệt giáo viên (2026-09-28).
//
// Middleware gắn TRỰC TIẾP từng route (không group.Use) — tránh lỗi H-01 (Use khớp PREFIX lan
// sang route khác đăng ký sau), cùng quy ước user_admin_router.go.
//
// PHẢI được gọi TRƯỚC SetupTeacherProfileRoutes (route.go): "/teacher-profiles/me" là segment
// tĩnh, nếu "/teacher-profiles/:id" đăng ký trước thì Fiber khớp :id="me" và trả sai.
func SetupApprovalRoutes(
	api fiber.Router,
	cfg *config.Config,
	approvalHandler *handler.ApprovalHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	auth := middleware.AuthMiddleware(cfg, redis)
	// Quyết định #3: chỉ SYSTEM_ADMIN duyệt khoá (COURSES_APPROVE_ALL; KHÔNG dùng _OWN_ORG).
	approveCourses := permChecker.RequirePermissions("COURSES_APPROVE_ALL")
	// Duyệt giáo viên bản chất là cấp role TEACHER — cùng quyền với gán/gỡ role hệ thống.
	manageRoles := permChecker.RequirePermissions("ROLES_MANAGE_SYSTEM")

	// Giáo viên chủ khoá nộp/nộp lại duyệt (kiểm chủ sở hữu trong repository).
	api.Post("/courses/:id/submit-review", auth, permChecker.RequirePermissions("COURSES_UPDATE_OWN"),
		approvalHandler.SubmitCourseForReview)

	api.Get("/admin/courses", auth, approveCourses, approvalHandler.ListCoursesForReview)
	api.Post("/admin/courses/:id/approve", auth, approveCourses, approvalHandler.ApproveCourse)
	api.Post("/admin/courses/:id/reject", auth, approveCourses, approvalHandler.RejectCourse)

	api.Get("/admin/teacher-applications", auth, manageRoles, approvalHandler.ListTeacherApplications)
	api.Post("/admin/teacher-applications/:userId/approve", auth, manageRoles, approvalHandler.ApproveTeacherApplication)
	api.Post("/admin/teacher-applications/:userId/reject", auth, manageRoles, approvalHandler.RejectTeacherApplication)

	// Người nộp hồ sơ tự xem trạng thái / nộp lại (quyết định #5).
	api.Get("/teacher-profiles/me", auth, approvalHandler.GetMyTeacherApplication)
	api.Post("/teacher-profiles/me/resubmit", auth, permChecker.RequirePermissions("TEACHER_PROFILE_UPDATE"),
		approvalHandler.ResubmitMyTeacherApplication)
}
