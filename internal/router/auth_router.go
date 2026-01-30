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
	auth.Post("/register/request", authHandler.RequestRegister)
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login) // done 
	auth.Post("/reset-password/request", authHandler.RequestPasswordReset)
	auth.Post("/reset-password", authHandler.ResetPassword)
	auth.Post("/refresh-token", authHandler.RefreshToken) // done

	
	auth.Use(middleware.AuthMiddleware(cfg, redis))
	
	auth.Get("/me", authHandler.GetMe)// done 
	auth.Get("/devices", authHandler.GetAllDevices) // Get all active devices
	auth.Post("/logout", authHandler.LogoutOneDevice) // done with logout one device
	auth.Post("/logout-all", authHandler.LogoutAll) // done with logout all devices
	auth.Put("/change-password", authHandler.ChangePassword) // done 
	// update profile  


	// updata
}
