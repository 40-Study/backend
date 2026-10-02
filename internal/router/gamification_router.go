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
// POST /achievements/:id/unlock - admin only (S5)
func SetupAchievementRoutes(api fiber.Router, cfg *config.Config, h *handler.AchievementHandler, redis *redis.Client, permChecker *middleware.PermissionChecker) {
	achievements := api.Group("/achievements")

	// Public: list all achievements
	achievements.Get("/", h.ListAchievements)

	// Auth-required routes
	auth := achievements.Use(middleware.AuthMiddleware(cfg, redis))
	auth.Get("/me", h.GetMyAchievements)
	// S5: route ghi chú "internal use" nhưng ai đăng nhập cũng tự mở khoá được mọi thành tựu (không kiểm điều kiện).
	// Chưa có luồng trao thành tựu nào khác và web chưa gọi; chỉ admin (SYSTEM_SETTINGS_MANAGE) cho tới khi có quyết định.
	auth.Post("/:id/unlock", permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"), h.UnlockAchievement)
}

// SetupLeaderboardRoutes registers /leaderboard endpoints.
// GET /leaderboard      - public
// GET /leaderboard/me   - auth required
func SetupLeaderboardRoutes(api fiber.Router, cfg *config.Config, h *handler.LeaderboardHandler, redis *redis.Client) {
	leaderboard := api.Group("/leaderboard")

	// Public nhưng theo người xem: OptionalAuth để biết chính chủ/admin (thấy tên thật) với khách và người khác
	// (người đặt leaderboard_display = anonymous hiện là "Học viên ẩn danh").
	leaderboard.Get("/", middleware.OptionalAuth(cfg, redis), h.GetLeaderboard)

	// Auth-required: current user rank
	leaderboard.Get("/me", middleware.AuthMiddleware(cfg, redis), h.GetMyRank)
}

// SetupUserStatsRoutes registers /users/:id/public-profile endpoint.
// GET /users/:id/public-profile - công khai nhưng theo người xem (S6): OptionalAuth để biết khách,
// chủ hồ sơ hay admin, rồi áp cài đặt riêng tư của chủ hồ sơ.
func SetupUserStatsRoutes(api fiber.Router, cfg *config.Config, h *handler.UserStatsHandler, redis *redis.Client) {
	api.Get("/users/:id/public-profile", middleware.OptionalAuth(cfg, redis), h.GetPublicProfile)
}
