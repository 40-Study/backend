package router

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

func SetupOpenDesignRoutes(api fiber.Router, cfg *config.Config, rdb *redis.Client) {
	h := handler.NewOpenDesignHandler(service.NewOpenDesignService(cfg))
	group := api.Group("/open-design", middleware.AuthMiddleware(cfg, rdb), func(c *fiber.Ctx) error {
		if c.Locals("active_role") != "TEACHER" {
			return c.Status(403).JSON(fiber.Map{"message": "Công cụ này chỉ dành cho giáo viên."})
		}
		return c.Next()
	})
	group.Get("/status", h.Status)
	group.Post("/documents", middleware.SameOriginRequired(cfg.ResolvedAllowedOrigins()), h.Document)
	group.Post("/generate", middleware.SameOriginRequired(cfg.ResolvedAllowedOrigins()), middleware.RateLimiter(rdb, middleware.RateLimitConfig{
		Max: 10, Window: time.Minute, KeyPrefix: "rate:open-design",
		KeyGenerator: func(c *fiber.Ctx) string { return fmt.Sprint(c.Locals("user_id")) },
		Message:      "Bạn đã gửi nhiều yêu cầu AI. Vui lòng chờ một phút.",
	}), h.Generate)
	group.Post("/cancel", middleware.SameOriginRequired(cfg.ResolvedAllowedOrigins()), h.Cancel)
}
