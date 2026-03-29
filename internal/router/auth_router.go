package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupAuthRoutes(api fiber.Router, cfg *config.Config, authHandler *handler.AuthHandler, redis *redis.Client) {
	auth := api.Group("/auth")
	// Public routes
	auth.Post("/register/request", authHandler.RequestRegister)
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/select-role", authHandler.SelectRole)
	auth.Get("/system-roles", authHandler.GetSystemRoleOptions)
	auth.Post("/reset-password/request", authHandler.RequestPasswordReset)
	auth.Post("/reset-password", authHandler.ResetPassword)
	auth.Post("/refresh-token", authHandler.RefreshToken)

	// Protected routes
	auth.Use(middleware.AuthMiddleware(cfg, redis))

	// Role management
	auth.Get("/my-roles", authHandler.GetMyRoles)
	auth.Post("/switch-role", authHandler.SwitchRole)

	// Profile management
	auth.Get("/me/profiles", authHandler.GetMyProfiles)
	auth.Post("/me/profiles", authHandler.CreateProfile)
	auth.Delete("/me/profiles/:id", authHandler.DeleteProfile)

	// User info
	auth.Get("/me", authHandler.GetMe)
	auth.Put("/me", authHandler.UpdateMe)

	// Session & devices
	auth.Get("/devices", authHandler.GetAllDevices)
	auth.Post("/logout", authHandler.LogoutOneDevice)
	auth.Post("/logout-all", authHandler.LogoutAll)
	auth.Put("/change-password", authHandler.ChangePassword)
}
