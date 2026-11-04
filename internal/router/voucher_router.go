package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
)

func SetupVoucherRoutes(api fiber.Router, cfg *config.Config, voucherHandler *handler.VoucherHandler, rdb *redis.Client, permChecker *middleware.PermissionChecker, auditRec middleware.AuditRecorder) {
	vouchers := api.Group("/vouchers")

	// Public routes
	vouchers.Get("/public", voucherHandler.GetPublicVouchers)
	// OptionalAuth: biết người xem (nếu có token) để voucher holders_only chỉ lộ cho người giữ nó.
	vouchers.Get("/code/:code", middleware.OptionalAuth(cfg, rdb), voucherHandler.GetVoucherByCode)

	// Protected routes - require auth
	auth := middleware.AuthMiddleware(cfg, rdb)
	vouchers.Get("/me", auth, voucherHandler.GetUserSavedVouchers)
	vouchers.Post("/:id/save", auth, voucherHandler.SaveVoucher)
	vouchers.Delete("/:id/save", auth, voucherHandler.UnsaveVoucher)

	// Admin routes - require auth + SYSTEM_SETTINGS_MANAGE
	// C-09 (audit 260909): comment nói "admin" nhưng chỉ gắn auth -> bất kỳ user đăng nhập
	// nào cũng tạo/sửa/xóa/kích hoạt được voucher (vd voucher giảm 100%).
	admin := permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE")
	// Id voucher mới nằm trong response, handler đặt đích bằng SetAuditTarget.
	vouchers.Post("/", auth, admin, middleware.Audit(auditRec, model.AuditActionVoucherCreate, "voucher", ""), voucherHandler.CreateVoucher)
	vouchers.Get("/", auth, admin, voucherHandler.GetAllVouchers)
	vouchers.Get("/:id", auth, admin, voucherHandler.GetVoucher)
	vouchers.Put("/:id", auth, admin, middleware.Audit(auditRec, model.AuditActionVoucherUpdate, "voucher", "id"), voucherHandler.UpdateVoucher)
	vouchers.Delete("/:id", auth, admin, middleware.Audit(auditRec, model.AuditActionVoucherDelete, "voucher", "id"), voucherHandler.DeleteVoucher)
	vouchers.Post("/:id/restore", auth, admin, middleware.Audit(auditRec, model.AuditActionVoucherRestore, "voucher", "id"), voucherHandler.RestoreVoucher)
	vouchers.Post("/:id/activate", auth, admin, middleware.Audit(auditRec, model.AuditActionVoucherActivate, "voucher", "id"), voucherHandler.ActivateVoucher)
	vouchers.Post("/:id/deactivate", auth, admin, middleware.Audit(auditRec, model.AuditActionVoucherDeactivate, "voucher", "id"), voucherHandler.DeactivateVoucher)
	vouchers.Get("/:id/stats", auth, admin, voucherHandler.GetVoucherStats)
}
