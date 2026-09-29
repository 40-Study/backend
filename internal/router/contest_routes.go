package router

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// contestUserRateLimit — giới hạn theo user_id (contract §2.2), đặt SAU auth nên luôn có user_id.
func contestUserRateLimit(rdb *redis.Client, prefix string, max int) fiber.Handler {
	return middleware.RateLimiter(rdb, middleware.RateLimitConfig{
		Max: max, Window: time.Minute, KeyPrefix: prefix,
		KeyGenerator: func(c *fiber.Ctx) string {
			id, _ := c.Locals("user_id").(uuid.UUID)
			return id.String()
		},
	})
}

// SetupContestRoutes — MVP "Cuộc thi" (contract §2.2 + ĐÍNH CHÍNH 28/09).
//
// C-02 (review vòng 5, vẫn giữ): KHÔNG dùng group + Use() — Fiber giữ MỘT stack middleware phẳng
// theo TIỀN TỐ nên Use() lan sang mọi route đăng ký sau ở cùng tiền tố (route công khai bị 401).
// Middleware gắn TRỰC TIẾP làm tham số của TỪNG route.
//
// I-04: các route TĨNH (/me, /manage, /manage/quiz-options, /manage/:id, /manage/:id/participants)
// PHẢI đăng ký TRƯỚC "/:slug" — Fiber khớp theo thứ tự đăng ký, không tự ưu tiên segment tĩnh.
//
// Route cũ đã GỠ (lộ đáp án/không có kiểm quyền): POST /:id/publish, GET|POST /:id/problems,
// PUT|DELETE /:id/problems/:problemId, POST /:id/problems/:problemId/submit, GET /:id/submissions/me.
func SetupContestRoutes(api fiber.Router, cfg *config.Config, h *handler.ContestHandler, rdb *redis.Client,
	permChecker *middleware.PermissionChecker) {
	auth := middleware.AuthMiddleware(cfg, rdb)
	optional := middleware.OptionalAuth(cfg, rdb)
	manage := permChecker.RequirePermissions("CONTESTS_MANAGE_OWN")
	approve := permChecker.RequirePermissions("CONTESTS_APPROVE_ALL")

	// ── Tĩnh (trước /:slug) ──
	api.Get("/contests", h.ListContests)
	api.Get("/contests/me", auth, h.GetMyContests)
	api.Get("/contests/manage", auth, manage, h.ListManage)
	api.Get("/contests/manage/quiz-options", auth, manage, h.QuizOptions)
	api.Get("/contests/manage/:id", auth, manage, h.GetManage)
	api.Get("/contests/manage/:id/participants", auth, manage, h.ListParticipants)

	// ── Công khai theo slug ──
	api.Get("/contests/:slug", optional, h.GetContest)

	// ── Giảng viên ──
	api.Post("/contests", auth, manage, h.CreateContest)
	api.Put("/contests/:id", auth, manage, h.UpdateContest)
	api.Delete("/contests/:id", auth, manage, h.DeleteContest)
	api.Post("/contests/:id/submit-review", auth, manage, h.SubmitReview)

	// ── Thí sinh ──
	api.Post("/contests/:id/join", auth, contestUserRateLimit(rdb, "rl:contest:join", 20), h.JoinContest)
	api.Post("/contests/:id/start", auth, contestUserRateLimit(rdb, "rl:contest:start", 10), h.StartContest)
	api.Post("/contests/:id/submit", auth, contestUserRateLimit(rdb, "rl:contest:submit", 10), h.SubmitContest)
	api.Get("/contests/:id/my-result", auth, h.MyResult)
	api.Get("/contests/:id/leaderboard", optional, h.Leaderboard)
	api.Get("/contests/:id/certificate", auth, h.Certificate)

	// ── Admin ──
	api.Get("/admin/contests", auth, approve, h.AdminListContests)
	api.Post("/admin/contests/:id/approve", auth, approve, h.ApproveContest)
	api.Post("/admin/contests/:id/reject", auth, approve, h.RejectContest)
	api.Post("/admin/contests/:id/cancel", auth, approve, h.CancelContest)
	api.Put("/admin/contests/:id/prizes", auth, approve, h.UpdatePrizes)
	api.Post("/admin/contests/:id/finalize", auth, approve, h.FinalizeContest)
}
