package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupPersonalEventRoutes(api fiber.Router, cfg *config.Config, h *handler.PersonalEventHandler, redis *redis.Client) {
	auth := middleware.AuthMiddleware(cfg, redis)

	me := api.Group("/me/events", auth)
	me.Post("/", h.Create)
	me.Get("/", h.List)
	me.Put("/:id", h.Update)
	me.Delete("/:id", h.Delete)
}
