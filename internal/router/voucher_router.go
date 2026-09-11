package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupVoucherRoutes(api fiber.Router, cfg *config.Config, voucherHandler *handler.VoucherHandler, rdb *redis.Client, permChecker *middleware.PermissionChecker) {
	vouchers := api.Group("/vouchers")

	// Public routes
	vouchers.Get("/public", voucherHandler.GetPublicVouchers)
	vouchers.Get("/code/:code", voucherHandler.GetVoucherByCode)

	// Protected routes - require auth
	auth := middleware.AuthMiddleware(cfg, rdb)
	vouchers.Get("/me", auth, voucherHandler.GetUserSavedVouchers)
	vouchers.Post("/:id/save", auth, voucherHandler.SaveVoucher)
	vouchers.Delete("/:id/save", auth, voucherHandler.UnsaveVoucher)

	// Admin routes - require auth + SYSTEM_SETTINGS_MANAGE
	// C-09 (audit 260909): comment nói "admin" nhưng chỉ gắn auth -> bất kỳ user đăng nhập
	// nào cũng tạo/sửa/xóa/kích hoạt được voucher (vd voucher giảm 100%).
	admin := permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE")
	vouchers.Post("/", auth, admin, voucherHandler.CreateVoucher)
	vouchers.Get("/", auth, admin, voucherHandler.GetAllVouchers)
	vouchers.Get("/:id", auth, admin, voucherHandler.GetVoucher)
	vouchers.Put("/:id", auth, admin, voucherHandler.UpdateVoucher)
	vouchers.Delete("/:id", auth, admin, voucherHandler.DeleteVoucher)
	vouchers.Post("/:id/restore", auth, admin, voucherHandler.RestoreVoucher)
	vouchers.Post("/:id/activate", auth, admin, voucherHandler.ActivateVoucher)
	vouchers.Post("/:id/deactivate", auth, admin, voucherHandler.DeactivateVoucher)
	vouchers.Get("/:id/stats", auth, admin, voucherHandler.GetVoucherStats)
}
