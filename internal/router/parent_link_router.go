package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupParentLinkRoutes — liên kết phụ huynh-học sinh do phụ huynh khởi xướng (QA vòng 2 lane E).
// Đặt dưới /family (prefix mới) để không dính middleware của group /parent sẵn có.
func SetupParentLinkRoutes(api fiber.Router, cfg *config.Config, h *handler.ParentLinkHandler, redis *redis.Client) {
	family := api.Group("/family")
	family.Use(middleware.AuthMiddleware(cfg, redis))

	// Phía phụ huynh
	family.Post("/link-requests", h.CreateRequest)
	family.Get("/link-requests/sent", h.ListSent)
	family.Post("/link-requests/:id/cancel", h.Cancel)
	family.Delete("/children/:id", h.UnlinkChild)

	// Phía học sinh
	family.Get("/link-requests/incoming", h.ListIncoming)
	family.Post("/link-requests/:id/respond", h.Respond)
	family.Get("/parents", h.ListLinkedParents)
	family.Delete("/parents/:id", h.UnlinkParent)
}
