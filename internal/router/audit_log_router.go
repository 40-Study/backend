package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupAuditLogRoutes: xem nhật ký hoạt động quản trị. Quyền SYSTEM_SETTINGS_MANAGE (plan D4,
// dùng chung với cài đặt hệ thống/thông báo hàng loạt; AUDIT_LOGS_VIEW để sau).
func SetupAuditLogRoutes(api fiber.Router,
	cfg *config.Config,
	h *handler.AuditLogHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker) {
	admin := api.Group("/admin/audit-logs",
		middleware.AuthMiddleware(cfg, redis),
		permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"))
	admin.Get("/", h.List)
	admin.Get("/actions", h.Actions)
}
