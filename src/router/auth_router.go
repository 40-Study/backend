package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"tiger.com/v2/src/config"
	"tiger.com/v2/src/handler"
	"tiger.com/v2/src/middleware"
)

func SetupAuthRoutes(api fiber.Router, cfg *config.Config, authHandler *handler.AuthHandler, redis *redis.Client) {
	auth := api.Group("/auth")
	auth.Post("/register/request", authHandler.RequestRegister)
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/reset-password/request", authHandler.RequestPasswordReset)
	auth.Post("/reset-password", authHandler.ResetPassword)
	auth.Post("/refresh-token", authHandler.RefreshToken)

	auth.Use(middleware.AuthMiddleware(cfg, redis))
	auth.Get("/me", authHandler.GetMe)
	auth.Post("/logout", authHandler.LogoutOneDevice)
	auth.Post("/logout-all", authHandler.LogoutAll)
	auth.Put("/change-password", authHandler.UpdatePasswordHash)
}
