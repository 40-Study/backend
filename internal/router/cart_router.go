package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupCartRoutes(
	api fiber.Router,
	cfg *config.Config,
	cartHandler *handler.CartHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// Cart routes - require authentication
	cart := api.Group("/cart", auth)
	{
		cart.Get("/", cartHandler.GetCart)
		cart.Post("/", cartHandler.AddToCart)
		cart.Delete("/", cartHandler.RemoveFromCart)
		cart.Delete("/clear", cartHandler.ClearCart)
		cart.Get("/check/:courseID", cartHandler.CheckCourseInCart)
	}
}
