package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupVoucherRoutes(api fiber.Router, voucherHandler *handler.VoucherHandler) {
	vouchers := api.Group("/vouchers")

	// Public routes
	vouchers.Get("/public", voucherHandler.GetPublicVouchers)
	vouchers.Get("/code/:code", voucherHandler.GetVoucherByCode)

	// Protected routes - require auth
	authMiddleware := middleware.AuthMiddleware(nil, nil)
	vouchers.Get("/me", authMiddleware, voucherHandler.GetUserSavedVouchers)
	vouchers.Post("/:id/save", authMiddleware, voucherHandler.SaveVoucher)
	vouchers.Delete("/:id/save", authMiddleware, voucherHandler.UnsaveVoucher)

	// Admin routes - require auth
	adminMiddleware := middleware.AuthMiddleware(nil, nil)
	vouchers.Post("/", adminMiddleware, voucherHandler.CreateVoucher)
	vouchers.Get("/", adminMiddleware, voucherHandler.GetAllVouchers)
	vouchers.Get("/:id", adminMiddleware, voucherHandler.GetVoucher)
	vouchers.Put("/:id", adminMiddleware, voucherHandler.UpdateVoucher)
	vouchers.Delete("/:id", adminMiddleware, voucherHandler.DeleteVoucher)
	vouchers.Post("/:id/restore", adminMiddleware, voucherHandler.RestoreVoucher)
	vouchers.Post("/:id/activate", adminMiddleware, voucherHandler.ActivateVoucher)
	vouchers.Post("/:id/deactivate", adminMiddleware, voucherHandler.DeactivateVoucher)
	vouchers.Get("/:id/stats", adminMiddleware, voucherHandler.GetVoucherStats)
}
