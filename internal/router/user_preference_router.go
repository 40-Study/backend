package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupUserPreferenceRoutes(api fiber.Router, cfg *config.Config, h *handler.UserPreferenceHandler, redis *redis.Client) {
	auth := middleware.AuthMiddleware(cfg, redis)
	prefs := api.Group("/preferences")
	prefs.Get("/privacy", auth, h.GetPrivacySettings)
	prefs.Put("/privacy", auth, h.UpdatePrivacySettings)
}
