package router

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
)

const (
	// adminBroadcastMaxPerHour: số lần gửi thông báo hệ thống tối đa mỗi quản trị viên mỗi giờ. Thông báo đã gửi
	// không thu hồi được, nên chặn bấm nhầm/lặp tay ở cả phía server (nút web cũng bị khoá khi đang gửi).
	adminBroadcastMaxPerHour = 5
	adminBroadcastRateWindow = time.Hour
)

// SetupAdminBroadcastRoutes — POST /admin/notifications/broadcast[/preview] (SYSTEM_SETTINGS_MANAGE, contract C3).
// Chỉ route gửi bị giới hạn tần suất; xem trước thì không (nó không gửi gì).
func SetupAdminBroadcastRoutes(api fiber.Router, cfg *config.Config, h *handler.AdminBroadcastHandler, rdb *redis.Client, permChecker *middleware.PermissionChecker, auditRec middleware.AuditRecorder) {
	admin := api.Group("/admin/notifications", middleware.AuthMiddleware(cfg, rdb), permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"))

	sendLimiter := middleware.RateLimiter(rdb, middleware.RateLimitConfig{
		Max:       adminBroadcastMaxPerHour,
		Window:    adminBroadcastRateWindow,
		KeyPrefix: "rate:admin-broadcast",
		// Khoá theo id quản trị viên (không theo IP): nhiều admin sau cùng một NAT không giành nhau hạn mức.
		KeyGenerator: func(c *fiber.Ctx) string {
			if id, ok := c.Locals("user_id").(uuid.UUID); ok {
				return id.String()
			}
			return "anonymous"
		},
		Message: fmt.Sprintf("Bạn đã gửi tối đa %d thông báo hệ thống trong 1 giờ. Vui lòng thử lại sau.", adminBroadcastMaxPerHour),
	})

	admin.Post("/broadcast/preview", h.Preview)
	// Audit đứng SAU limiter (429 không ghi) và chỉ route gửi (preview không). Handler tự bật ghi cả khi gửi dở dang (500 BROADCAST_PARTIAL).
	admin.Post("/broadcast", sendLimiter, middleware.Audit(auditRec, model.AuditActionNotificationBroadcast, "", ""), h.Send)
}
