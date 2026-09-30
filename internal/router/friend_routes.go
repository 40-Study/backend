package router

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// userRateLimit — giới hạn theo user_id (đặt SAU auth nên luôn có user_id) với cửa sổ tuỳ chọn. Redis lỗi
// thì 503 fail-closed như mọi RateLimiter khác. Bản cùng ý với contestUserRateLimit nhưng có tham số
// Window; việc gộp hai hàm là ticket riêng để không sửa contest_routes.go trong đợt này.
func userRateLimit(rdb *redis.Client, prefix string, max int, window time.Duration) fiber.Handler {
	return middleware.RateLimiter(rdb, middleware.RateLimitConfig{
		Max: max, Window: window, KeyPrefix: prefix,
		KeyGenerator: func(c *fiber.Ctx) string {
			id, _ := c.Locals("user_id").(uuid.UUID)
			return id.String()
		},
	})
}

// SetupFriendRoutes — /api/friends (contract-api.md §1).
//
// Không dùng group + Use() (bẫy H-01, xem user_system_role_router.go): Fiber giữ một stack middleware
// phẳng theo tiền tố nên Use() lan sang route đăng ký sau. Middleware gắn TRỰC TIẾP vào từng route.
// Route tĩnh (summary, requests, search, blocks, relationship) đăng ký TRƯỚC "/:userId".
//
// Giới hạn tốc độ: mọi POST dùng chung một bộ đếm 10/phút/user; search có bộ đếm riêng 30/phút/user.
func SetupFriendRoutes(api fiber.Router, cfg *config.Config, h *handler.FriendshipHandler, rdb *redis.Client) {
	auth := middleware.AuthMiddleware(cfg, rdb)
	post := userRateLimit(rdb, "rl:friends:post", constants.FriendPostsPerMinute, time.Minute)
	search := userRateLimit(rdb, "rl:friends:search", constants.FriendSearchPerMinute, time.Minute)

	api.Get("/friends", auth, h.ListFriends)
	api.Get("/friends/summary", auth, h.Summary)
	api.Get("/friends/requests", auth, h.ListRequests)
	api.Post("/friends/requests", auth, post, h.SendRequest)
	api.Post("/friends/requests/:id/accept", auth, post, h.AcceptRequest)
	api.Post("/friends/requests/:id/decline", auth, post, h.DeclineRequest)
	api.Delete("/friends/requests/:id", auth, h.CancelRequest)
	api.Get("/friends/search", auth, search, h.Search)
	api.Get("/friends/relationship/:userId", auth, h.Relationship)
	api.Get("/friends/blocks", auth, h.ListBlocks)
	api.Post("/friends/blocks", auth, post, h.Block)
	api.Delete("/friends/blocks/:userId", auth, h.Unblock)
	api.Delete("/friends/:userId", auth, h.Unfriend)
}
