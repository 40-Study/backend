package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupCoinRoutes(api fiber.Router, cfg *config.Config, h *handler.CoinHandler, redis *redis.Client, permChecker *middleware.PermissionChecker) {
	coins := api.Group("/coins")
	auth := middleware.AuthMiddleware(cfg, redis)

	// Public - packages listing
	coins.Get("/packages", h.ListPackages)
	coins.Get("/packages/:id", h.GetPackage)

	// Auth required
	authed := coins.Group("")
	authed.Use(auth)

	// Wallet
	authed.Get("/wallet", h.GetWallet)
	authed.Get("/wallet/transactions", h.GetTransactions)

	// Purchases
	authed.Post("/purchases", h.CreatePurchase)
	authed.Get("/purchases", h.ListPurchases)
	authed.Get("/purchases/:id", h.GetPurchase)
	authed.Post("/purchases/:id/verify", h.VerifyPurchase)

	// Gift
	authed.Post("/gift", h.SendGift)

	// Admin
	// C-08 (audit 260909): trước đây chỉ có AuthMiddleware -> bất kỳ user nào cũng tự cộng
	// xu qua /admin/adjust hoặc tự tạo gói xu. Gắn thêm permission SYSTEM_SETTINGS_MANAGE
	// (chỉ SYSTEM_ADMIN có, qua wildcard "*").
	admin := coins.Group("/admin")
	admin.Use(auth)
	admin.Use(permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"))
	admin.Post("/packages", h.CreatePackage)
	admin.Put("/packages/:id", h.UpdatePackage)
	admin.Delete("/packages/:id", h.DeletePackage)
	admin.Post("/adjust", h.AdminAdjust)
}
