package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupAdminSettingsRoutes — GET /admin/settings (contract C4, quyền SYSTEM_SETTINGS_MANAGE).
//
// Middleware gắn trực tiếp trên route (không group.Use): Use() khớp theo PREFIX nên một group
// "/admin/settings" sẽ chặn cả các route đăng ký sau dưới tiền tố đó (lỗi H-01, xem
// withdrawal_router.go). Đường ghi PUT /admin/settings/platform-fee nằm ở SetupOrderRoutes.
func SetupAdminSettingsRoutes(
	api fiber.Router,
	cfg *config.Config,
	h *handler.AdminSettingsHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	api.Get("/admin/settings",
		middleware.AuthMiddleware(cfg, redis),
		permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"),
		h.GetSettings)
}
