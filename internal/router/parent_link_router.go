package router

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// parentLinkIPMaxPerHour — số lần gửi yêu cầu liên kết tối đa mỗi IP mỗi giờ (gộp mọi tài khoản).
const parentLinkIPMaxPerHour = 20

// SetupParentLinkRoutes — liên kết phụ huynh-học sinh do phụ huynh khởi xướng (QA vòng 2 lane E).
// Đặt dưới /family (prefix mới) để không dính middleware của group /parent sẵn có.
func SetupParentLinkRoutes(api fiber.Router, cfg *config.Config, h *handler.ParentLinkHandler, redis *redis.Client) {
	family := api.Group("/family")
	family.Use(middleware.AuthMiddleware(cfg, redis))

	// Review PR #81, MAJOR-1: ngoài hạn mức theo tài khoản (service), chặn thêm theo IP để một
	// người không dùng nhiều tài khoản phụ huynh từ cùng máy để dò email học sinh.
	linkRequestIPLimiter := middleware.RateLimiter(redis, middleware.RateLimitConfig{
		Max:            parentLinkIPMaxPerHour,
		Window:         time.Hour,
		KeyPrefix:      "rate:family-link-request",
		Message:        "Bạn gửi yêu cầu liên kết quá nhiều lần từ thiết bị này. Vui lòng thử lại sau.",
		TrustedProxies: middleware.NewTrustedProxySet(cfg.ResolvedTrustedProxies()),
	})

	// Phía phụ huynh
	family.Post("/link-requests", linkRequestIPLimiter, h.CreateRequest)
	family.Get("/link-requests/sent", h.ListSent)
	family.Post("/link-requests/:id/cancel", h.Cancel)
	family.Delete("/children/:id", h.UnlinkChild)

	// Phía học sinh
	family.Get("/link-requests/incoming", h.ListIncoming)
	family.Post("/link-requests/:id/respond", h.Respond)
	family.Get("/parents", h.ListLinkedParents)
	family.Delete("/parents/:id", h.UnlinkParent)
}
