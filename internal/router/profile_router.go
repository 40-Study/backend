package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupProfileRoutes(api fiber.Router, cfg *config.Config, profileHandler *handler.ProfileHandler, redis *redis.Client) {
	me := api.Group("/me", middleware.AuthMiddleware(cfg, redis))

	me.Get("/children", profileHandler.GetChildren)
	me.Get("/parents", profileHandler.GetParents)
	me.Get("/organizations", profileHandler.GetOrganizations)
	me.Get("/org-roles", profileHandler.GetMyOrgRoles)
}
