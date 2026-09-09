package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupContestRoutes(api fiber.Router, cfg *config.Config, h *handler.ContestHandler, redis *redis.Client) {
	contests := api.Group("/contests")

	auth := middleware.AuthMiddleware(cfg, redis)

	// Public
	contests.Get("/", h.ListContests)

	// Auth required
	authed := contests.Group("")
	authed.Use(auth)

	// My contests (I-04, review vòng 4/5): PHẢI đăng ký TRƯỚC "/:slug" bên dưới — cùng tiền tố
	// "/contests" (authed := contests.Group("")), Fiber khớp theo THỨ TỰ ĐĂNG KÝ khi route tham
	// số và route tĩnh cùng độ sâu, không tự ưu tiên tĩnh trước tham số (đã tự kiểm chứng cùng
	// lớp lỗi ở quiz_router.go/grade_router.go, review vòng 3/4). TRƯỚC ĐÂY "/:slug" đăng ký
	// trước "/me" nên GET /api/contests/me luôn rơi vào GetContest(slug="me"),
	// GetMyContests là code chết vĩnh viễn dù không lỗi biên dịch/runtime nào báo.
	authed.Get("/me", h.GetMyContests)

	// Public slug lookup — ĐĂNG KÝ SAU "/me" ở trên (xem comment).
	contests.Get("/:slug", h.GetContest)

	// Contest CRUD
	authed.Post("/", h.CreateContest)
	authed.Put("/:id", h.UpdateContest)
	authed.Delete("/:id", h.DeleteContest)
	authed.Post("/:id/publish", h.PublishContest)

	// Problems
	authed.Get("/:id/problems", h.GetProblems)
	authed.Post("/:id/problems", h.CreateProblem)
	authed.Put("/:id/problems/:problemId", h.UpdateProblem)
	authed.Delete("/:id/problems/:problemId", h.DeleteProblem)

	// Participation
	authed.Post("/:id/join", h.JoinContest)
	authed.Get("/:id/leaderboard", h.GetLeaderboard)

	// Submissions
	authed.Post("/:id/problems/:problemId/submit", h.SubmitAnswer)
	authed.Get("/:id/submissions/me", h.GetMySubmissions)
}
