package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupProfileRoutes(api fiber.Router, cfg *config.Config, profileHandler *handler.ProfileHandler, redis *redis.Client) {
	profile := api.Group("/profile")

	// Tất cả routes trong /profile đều cần authentication
	profile.Use(middleware.AuthMiddleware(cfg, redis))

	profile.Get("/children", profileHandler.GetChildren)

	profile.Get("/organizations", profileHandler.GetOrganizations)
	profile.Get("/org-roles", profileHandler.GetMyOrgRoles)
}
