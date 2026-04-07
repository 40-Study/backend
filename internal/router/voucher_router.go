package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupVoucherRoutes(api fiber.Router, cfg *config.Config, voucherHandler *handler.VoucherHandler, rdb *redis.Client) {
	vouchers := api.Group("/vouchers")

	// Public routes
	vouchers.Get("/public", voucherHandler.GetPublicVouchers)
	vouchers.Get("/code/:code", voucherHandler.GetVoucherByCode)

	// Protected routes - require auth
	auth := middleware.AuthMiddleware(cfg, rdb)
	vouchers.Get("/me", auth, voucherHandler.GetUserSavedVouchers)
	vouchers.Post("/:id/save", auth, voucherHandler.SaveVoucher)
	vouchers.Delete("/:id/save", auth, voucherHandler.UnsaveVoucher)

	// Admin routes - require auth
	vouchers.Post("/", auth, voucherHandler.CreateVoucher)
	vouchers.Get("/", auth, voucherHandler.GetAllVouchers)
	vouchers.Get("/:id", auth, voucherHandler.GetVoucher)
	vouchers.Put("/:id", auth, voucherHandler.UpdateVoucher)
	vouchers.Delete("/:id", auth, voucherHandler.DeleteVoucher)
	vouchers.Post("/:id/restore", auth, voucherHandler.RestoreVoucher)
	vouchers.Post("/:id/activate", auth, voucherHandler.ActivateVoucher)
	vouchers.Post("/:id/deactivate", auth, voucherHandler.DeactivateVoucher)
	vouchers.Get("/:id/stats", auth, voucherHandler.GetVoucherStats)
}
