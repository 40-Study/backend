package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupWalletRoutes(
	api fiber.Router,
	cfg *config.Config,
	walletHandler *handler.WalletHandler,
	redis *redis.Client,
) {
	authMiddleware := middleware.AuthMiddleware(cfg, redis)

	wallet := api.Group("/wallet")
	wallet.Get("/me", authMiddleware, walletHandler.GetWallet)
	wallet.Get("/transactions", authMiddleware, walletHandler.GetTransactions)
}
