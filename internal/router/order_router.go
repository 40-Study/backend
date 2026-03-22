package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupOrderRoutes(api fiber.Router,
	cfg *config.Config,
	orderHandler *handler.OrderHandler,
	redis *redis.Client) {
	orders := api.Group("/orders")

	authMiddleware := middleware.AuthMiddleware(nil, nil)
	orders.Post("/", authMiddleware, orderHandler.CreateOrder)
	orders.Get("/me", authMiddleware, orderHandler.GetUserOrders)
	orders.Get("/:id", authMiddleware, orderHandler.GetOrder)
	orders.Post("/:id/cancel", authMiddleware, orderHandler.CancelOrder)
	orders.Post("/:id/payment-intent", authMiddleware, orderHandler.CreatePaymentIntent)
	orders.Get("/:id/payment-status", authMiddleware, orderHandler.GetPaymentStatus)
	orders.Post("/:id/check-payment", authMiddleware, orderHandler.CheckPayment)
}
