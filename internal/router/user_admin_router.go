package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupUserAdminRoutes — Phase 1 quản lý người dùng (2026-09-28). Đứng ở group "/users" riêng
// (không dùng chung group với user_system_role_router.go / user_organization_role_router.go)
// để tránh lặp lại lỗi H-01 (group.Use() khớp PREFIX ảnh hưởng route đăng ký sau) — gắn
// middleware trực tiếp lên TỪNG route, giống quy ước đã sửa ở 2 router kia.
func SetupUserAdminRoutes(
	api fiber.Router,
	cfg *config.Config,
	userAdminHandler *handler.UserAdminHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	auth := middleware.AuthMiddleware(cfg, redis)
	viewPerm := permChecker.RequirePermissions("USERS_VIEW_ALL")
	banPerm := permChecker.RequirePermissions("USERS_BAN")

	users := api.Group("/users")

	// GET /users — danh sách + tìm + lọc + phân trang
	users.Get("/", auth, viewPerm, userAdminHandler.GetUsers)
	// GET /users/:id — chi tiết 1 user
	users.Get("/:id", auth, viewPerm, userAdminHandler.GetUser)
	// PUT /users/:id/status — khoá / mở khoá
	users.Put("/:id/status", auth, banPerm, userAdminHandler.UpdateUserStatus)
}
