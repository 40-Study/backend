package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupProfileRoutes(api fiber.Router, cfg *config.Config, profileHandler *handler.ProfileHandler, redis *redis.Client) {
	profile := api.Group("/profile")

	// Tất cả routes trong /profile đều cần authentication
	profile.Use(middleware.AuthMiddleware(cfg, redis))

	// GET /profile/children - Lấy danh sách con của phụ huynh (role PARENT)
	profile.Get("/children", profileHandler.GetChildren)

	// GET /profile/organizations - Lấy danh sách tổ chức của user (role ORG_OWNER hoặc member)
	profile.Get("/organizations", profileHandler.GetOrganizations)
}
