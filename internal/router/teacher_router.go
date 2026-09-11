package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupTeacherRoutes(
	api fiber.Router,
	cfg *config.Config,
	teacherHandler *handler.TeacherHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	teachers := api.Group("/teachers")
	{
		teachers.Get("/", teacherHandler.GetAllTeachers)
		teachers.Get("/me/students", middleware.AuthMiddleware(cfg, redis), teacherHandler.GetMyStudents)
		teachers.Get("/:id", teacherHandler.GetTeacher)
		// H-06 (review vòng 1): route này TRƯỚC ĐÂY không có AuthMiddleware lẫn permission —
		// bất kỳ ai (kể cả chưa đăng nhập) gọi được, và nó xóa luôn UserSystemRole của người đó
		// (teacher_repository.go). Gate bằng permission quản trị hệ thống, cùng nhóm với các
		// route admin khác (coin/voucher/upload) — xem permission_helper.go.
		teachers.Delete("/:id", middleware.AuthMiddleware(cfg, redis), permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"), teacherHandler.DeleteTeacher)
	}
}
