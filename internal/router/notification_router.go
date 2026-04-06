package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupNotificationRoutes(api fiber.Router, cfg *config.Config, h *handler.NotificationHandler, redis *redis.Client) {
	notifications := api.Group("/notifications")
	notifications.Use(middleware.AuthMiddleware(cfg, redis))

	notifications.Get("/", h.ListNotifications)
	notifications.Get("/unread-count", h.GetUnreadCount)
	notifications.Patch("/read-all", h.MarkAllAsRead)
	notifications.Patch("/:id/read", h.MarkAsRead)
	notifications.Delete("/:id", h.DeleteNotification)
	notifications.Post("/send", h.SendNotification)
}
