package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
)

func SetupOrderRoutes(api fiber.Router,
	cfg *config.Config,
	orderHandler *handler.OrderHandler,
	adminOrderHandler *handler.AdminOrderHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
	auditRec middleware.AuditRecorder) {
	orders := api.Group("/orders")

	// H-01 (audit 260909): AuthMiddleware(nil, nil) khiến utils.ParseToken truy cập
	// cfg.JWTSecret trên cfg == nil -> nil pointer dereference, panic mọi request có token.
	// cfg/redis đã có sẵn trong tham số hàm nhưng không được dùng — sửa lại cho đúng.
	authMiddleware := middleware.AuthMiddleware(cfg, redis)

	// Đơn hàng admin + hoàn tiền (phase-02-orders-refund.md): nhóm "/admin" PHẢI đăng ký TRƯỚC
	// "/:id" bên dưới — Fiber khớp route theo THỨ TỰ đăng ký, "/:id" đứng trước sẽ "nuốt" mất
	// "/admin" (khớp :id="admin"). Quyền PAYMENTS_MANAGE (permission mới — xem
	// data/permissions/system_admin_permissions.json) — V1 chỉ SYSTEM_ADMIN xem được (role đó có
	// "*", seeder tự mở rộng thành mọi permission, xem seeds/seeder.go).
	adminOrders := orders.Group("/admin", authMiddleware, permChecker.RequirePermissions("PAYMENTS_MANAGE"))
	adminOrders.Get("/", adminOrderHandler.ListOrders)
	adminOrders.Get("/:id", adminOrderHandler.GetOrder)
	adminOrders.Post("/:id/refund", middleware.Audit(auditRec, model.AuditActionOrderRefund, "order", "id"), adminOrderHandler.RefundOrder)
	// Đánh dấu "đã hoàn tiền xong" cho khoản tiền về muộn của đơn đã đóng (cờ refund_needed).
	adminOrders.Post("/:id/late-refund", middleware.Audit(auditRec, model.AuditActionOrderLateRefund, "order", "id"), adminOrderHandler.MarkLatePaymentRefunded)

	orders.Post("/", authMiddleware, orderHandler.CreateOrder)
	orders.Get("/me", authMiddleware, orderHandler.GetUserOrders)
	orders.Get("/:id", authMiddleware, orderHandler.GetOrder)
	orders.Post("/:id/cancel", authMiddleware, orderHandler.CancelOrder)
	orders.Post("/:id/payment-intent", authMiddleware, orderHandler.CreatePaymentIntent)
	orders.Get("/:id/payment-status", authMiddleware, orderHandler.GetPaymentStatus)
	orders.Post("/:id/check-payment", authMiddleware, orderHandler.CheckPayment)

	// Báo cáo doanh thu nền tảng thật (DASHBOARD_VIEW_GLOBAL) + cấu hình % phí nền tảng
	// (SYSTEM_SETTINGS_MANAGE, quyết định #2) — cùng nhóm "/admin" gốc (không phải "/orders").
	adminReports := api.Group("/admin/reports", authMiddleware, permChecker.RequirePermissions("DASHBOARD_VIEW_GLOBAL"))
	adminReports.Get("/revenue", adminOrderHandler.GetRevenueReport)

	adminSettings := api.Group("/admin/settings", authMiddleware, permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"))
	adminSettings.Get("/platform-fee", adminOrderHandler.GetPlatformFeeSetting)
	// Đích "platform_fee" và metadata {old,new} do handler đặt (SetAuditTarget/SetAuditMeta).
	adminSettings.Put("/platform-fee", middleware.Audit(auditRec, model.AuditActionSettingPlatformFeeUpdate, "setting", ""), adminOrderHandler.UpdatePlatformFeeSetting)
}
