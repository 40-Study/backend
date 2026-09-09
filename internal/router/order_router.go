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

	// H-01 (audit 260909): AuthMiddleware(nil, nil) khiến utils.ParseToken truy cập
	// cfg.JWTSecret trên cfg == nil -> nil pointer dereference, panic mọi request có token.
	// cfg/redis đã có sẵn trong tham số hàm nhưng không được dùng — sửa lại cho đúng.
	authMiddleware := middleware.AuthMiddleware(cfg, redis)
	orders.Post("/", authMiddleware, orderHandler.CreateOrder)
	orders.Get("/me", authMiddleware, orderHandler.GetUserOrders)
	orders.Get("/:id", authMiddleware, orderHandler.GetOrder)
	orders.Post("/:id/cancel", authMiddleware, orderHandler.CancelOrder)
	orders.Post("/:id/payment-intent", authMiddleware, orderHandler.CreatePaymentIntent)
	orders.Get("/:id/payment-status", authMiddleware, orderHandler.GetPaymentStatus)
	orders.Post("/:id/check-payment", authMiddleware, orderHandler.CheckPayment)
}
