package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupTeacherProfileRoutes đăng ký route hồ sơ giáo viên.
//
// C-05 (audit 260909): trước đây hàm này không nhận cfg/redis nên KHÔNG THỂ gắn
// AuthMiddleware — ẩn danh tạo/sửa/xóa được hồ sơ giáo viên bất kỳ. Đã thêm cfg, redis;
// kiểm tra chủ sở hữu (profile.UserID == user_id) được thực hiện ở TeacherProfileService.
func SetupTeacherProfileRoutes(
	api fiber.Router,
	cfg *config.Config,
	teacherProfileHandler *handler.TeacherProfileHandler,
	redis *redis.Client,
) {
	authMiddleware := middleware.AuthMiddleware(cfg, redis)

	profiles := api.Group("/teacher-profiles")
	{
		profiles.Post("/", authMiddleware, teacherProfileHandler.CreateTeacherProfile)
		profiles.Get("/", teacherProfileHandler.GetAllTeacherProfiles)
		profiles.Get("/:id", teacherProfileHandler.GetTeacherProfileByID)
		profiles.Put("/:id", authMiddleware, teacherProfileHandler.UpdateTeacherProfile)
		profiles.Delete("/:id", authMiddleware, teacherProfileHandler.DeleteTeacherProfile)
	}
}
