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
	// adminBroadcastIdempotencyTTL: cùng Idempotency-Key của cùng admin trong khoảng này trả lại kết quả gốc (M2).
	adminBroadcastIdempotencyTTL = 24 * time.Hour
	// adminBroadcastIdempotencyPendingTTL: trần thời gian một lần gửi còn được coi là "đang chạy" (phòng tiến trình chết).
	adminBroadcastIdempotencyPendingTTL = 10 * time.Minute
)

// broadcastDelivered: lần gửi này đã giao >= 1 thông báo (thành công, hoặc dở dang rồi mới lỗi). Handler ghi số người đã
// nhận vào Locals; lỗi validate, NO_RECIPIENTS và lỗi trước khi giao ai không để lại gì. SSOT cho hai thứ cùng hỏi
// "lần này có tác dụng không": hoàn hạn mức (M4) và lưu kết quả để phát lại theo Idempotency-Key (M2).
func broadcastDelivered(c *fiber.Ctx) bool {
	n, _ := c.Locals(handler.BroadcastDeliveredLocal).(int64)
	return n > 0
}

// SetupAdminBroadcastRoutes — POST /admin/notifications/broadcast[/preview] (SYSTEM_SETTINGS_MANAGE, contract C3).
// Chỉ route gửi bị giới hạn tần suất (và nhận Idempotency-Key); xem trước thì không (nó không gửi gì).
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
		// M4: chỉ lần gửi thực sự giao >= 1 thông báo mới tiêu hạn mức.
		Refund: func(c *fiber.Ctx) bool { return !broadcastDelivered(c) },
	})

	// M2: Idempotency-Key (tuỳ chọn). Đứng TRƯỚC limiter và audit để bản phát lại không tiêu hạn mức, không bị 429 và
	// không ghi nhật ký lần hai.
	idempotent := middleware.Idempotency(rdb, middleware.IdempotencyConfig{
		KeyPrefix:  "idem:admin-broadcast",
		TTL:        adminBroadcastIdempotencyTTL,
		PendingTTL: adminBroadcastIdempotencyPendingTTL,
		Scope: func(c *fiber.Ctx) string {
			if id, ok := c.Locals("user_id").(uuid.UUID); ok {
				return id.String()
			}
			return ""
		},
		Took: broadcastDelivered,
	})

	admin.Post("/broadcast/preview", h.Preview)
	// Audit đứng SAU limiter (429 không ghi) và chỉ route gửi (preview không). Handler tự bật ghi cả khi gửi dở dang (500 BROADCAST_PARTIAL).
	admin.Post("/broadcast", idempotent, sendLimiter, middleware.Audit(auditRec, model.AuditActionNotificationBroadcast, "", ""), h.Send)
}
