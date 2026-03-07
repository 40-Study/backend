package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupLiveRouter(api fiber.Router, cfg *config.Config, liveHandler *handler.LiveHandler, redis *redis.Client) {
	auth := api.Group("/live")
	auth.Use(middleware.AuthMiddleware(cfg, redis))

	auth.Post("/create-token", liveHandler.CreateToken)
}
