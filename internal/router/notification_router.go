package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupNotificationRoutes(api fiber.Router, cfg *config.Config, h *handler.NotificationHandler, redis *redis.Client, permChecker *middleware.PermissionChecker) {
	notifications := api.Group("/notifications")
	notifications.Use(middleware.AuthMiddleware(cfg, redis))

	notifications.Get("/", h.ListNotifications)
	notifications.Get("/unread-count", h.GetUnreadCount)
	notifications.Get("/settings", h.GetSettings)
	notifications.Put("/settings", h.UpdateSettings)
	notifications.Patch("/read-all", h.MarkAllAsRead)
	notifications.Patch("/:id/read", h.MarkAsRead)
	notifications.Delete("/:id", h.DeleteNotification)
	// S5: gửi thông báo tới user_ids TUỲ Ý (giả mạo, spam) trước đây chỉ cần đăng nhập. Chưa có luồng nào của web gọi;
	// thông báo hệ thống đi qua service, không qua route này. Chỉ admin (SYSTEM_SETTINGS_MANAGE); ai khác cần gửi là câu hỏi mở.
	notifications.Post("/send", permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"), h.SendNotification)
}
