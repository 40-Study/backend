package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupAchievementRoutes registers /achievements endpoints.
// GET /achievements        - public list
// GET /achievements/me     - auth required
// POST /achievements/:id/unlock - auth required (internal use)
func SetupAchievementRoutes(api fiber.Router, cfg *config.Config, h *handler.AchievementHandler, redis *redis.Client) {
	achievements := api.Group("/achievements")

	// Public: list all achievements
	achievements.Get("/", h.ListAchievements)

	// Auth-required routes
	auth := achievements.Use(middleware.AuthMiddleware(cfg, redis))
	auth.Get("/me", h.GetMyAchievements)
	auth.Post("/:id/unlock", h.UnlockAchievement)
}

// SetupLeaderboardRoutes registers /leaderboard endpoints.
// GET /leaderboard      - public
// GET /leaderboard/me   - auth required
func SetupLeaderboardRoutes(api fiber.Router, cfg *config.Config, h *handler.LeaderboardHandler, redis *redis.Client) {
	leaderboard := api.Group("/leaderboard")

	// Public: top users
	leaderboard.Get("/", h.GetLeaderboard)

	// Auth-required: current user rank
	leaderboard.Get("/me", middleware.AuthMiddleware(cfg, redis), h.GetMyRank)
}

// SetupUserStatsRoutes registers /users/:id/public-profile endpoint.
// GET /users/:id/public-profile - public
func SetupUserStatsRoutes(api fiber.Router, h *handler.UserStatsHandler) {
	api.Get("/users/:id/public-profile", h.GetPublicProfile)
}
