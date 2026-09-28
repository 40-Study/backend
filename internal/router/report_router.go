package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupReportRoutes(
	api fiber.Router,
	cfg *config.Config,
	reportHandler *handler.ReportHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// A-P0-1 (QA 260927): trước đây chỉ có `auth` trên cả nhóm — bất kỳ user đã đăng nhập nào
	// cũng liệt kê được TOÀN BỘ report, đổi trạng thái, hoặc xoá report của người khác. Tạo report
	// và xem report của chính mình vẫn mở cho mọi user đã đăng nhập; kiểm duyệt (liệt kê tất cả,
	// đổi trạng thái, xoá) chỉ dành cho admin hệ thống (REPORTS_MODERATE, chỉ SYSTEM_ADMIN có qua
	// wildcard "*" — xem data/permissions/system_admin_permissions.json).
	requireModerate := permChecker.RequirePermissions("REPORTS_MODERATE")

	reports := api.Group("/reports", auth)
	{
		reports.Post("/", reportHandler.CreateReport)
		reports.Get("/", requireModerate, reportHandler.ListReports)
		reports.Get("/my", reportHandler.GetMyReports)
		reports.Get("/:id", reportHandler.GetReportByID)
		reports.Put("/:id/status", requireModerate, reportHandler.UpdateReportStatus)
		reports.Delete("/:id", requireModerate, reportHandler.DeleteReport)
	}
}
