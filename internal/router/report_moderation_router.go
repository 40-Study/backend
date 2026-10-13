package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
)

// SetupReportModerationRoutes (ADM-N4) - POST /reports/:id/actions. Cùng quyền REPORTS_MODERATE với các route kiểm duyệt khác
// ở report_router.go; đăng ký riêng ở app.go (qa2_be3_routes.go) như wishlist. Middleware gắn trực tiếp lên route
// (không group.Use) để không lan sang route khác cùng prefix (bài học H-01).
func SetupReportModerationRoutes(
	api fiber.Router,
	cfg *config.Config,
	h *handler.ReportModerationHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
	auditRec middleware.AuditRecorder,
) {
	auth := middleware.AuthMiddleware(cfg, redis)
	requireModerate := permChecker.RequirePermissions("REPORTS_MODERATE")
	api.Post("/reports/:id/actions", auth, requireModerate,
		middleware.Audit(auditRec, model.AuditActionReportModerate, "report", "id"), h.ApplyAction)
}
