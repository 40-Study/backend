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

	// C-02 (review vòng 5→6): TRƯỚC ĐÂY dùng `authed := contests.Group(""); authed.Use(auth)` —
	// Fiber giữ MỘT stack middleware PHẲNG theo TIỀN TỐ ("/contests"), không theo biến Go dùng để
	// đăng ký route. `authed.Use(auth)` áp dụng cho MỌI route đăng ký SAU nó ở cùng tiền tố, KỂ CẢ
	// route đăng ký qua biến `contests` (không phải `authed`) — nên `contests.Get("/:slug", ...)`
	// dù cố tình đăng ký qua biến "public" vẫn bị auth chặn (public route trả 401). Đo bằng Fiber
	// thật xác nhận đúng cơ chế này (xem báo cáo review vòng 5). Sửa: KHÔNG dùng group + Use() cho
	// cặp route công khai/riêng tư xen kẽ này — gắn `auth` TRỰC TIẾP làm middleware tham số cho
	// TỪNG route cần bảo vệ, "/:slug" không nhận middleware nào nên luôn công khai bất kể thứ tự
	// đăng ký các route khác.
	//
	// I-04 (review vòng 4/5, vẫn giữ nguyên): "/me" PHẢI đăng ký TRƯỚC "/:slug" — cùng tiền tố,
	// Fiber khớp theo THỨ TỰ ĐĂNG KÝ khi route tham số và route tĩnh cùng độ sâu, không tự ưu
	// tiên tĩnh trước tham số (đã tự kiểm chứng cùng lớp lỗi ở quiz_router.go/grade_router.go).
	contests.Get("/me", auth, h.GetMyContests)

	// Public slug lookup — ĐĂNG KÝ SAU "/me" ở trên, KHÔNG có middleware auth (xem comment C-02).
	contests.Get("/:slug", h.GetContest)

	// Contest CRUD
	contests.Post("/", auth, h.CreateContest)
	contests.Put("/:id", auth, h.UpdateContest)
	contests.Delete("/:id", auth, h.DeleteContest)
	contests.Post("/:id/publish", auth, h.PublishContest)

	// Problems
	contests.Get("/:id/problems", auth, h.GetProblems)
	contests.Post("/:id/problems", auth, h.CreateProblem)
	contests.Put("/:id/problems/:problemId", auth, h.UpdateProblem)
	contests.Delete("/:id/problems/:problemId", auth, h.DeleteProblem)

	// Participation
	contests.Post("/:id/join", auth, h.JoinContest)
	contests.Get("/:id/leaderboard", auth, h.GetLeaderboard)

	// Submissions
	contests.Post("/:id/problems/:problemId/submit", auth, h.SubmitAnswer)
	contests.Get("/:id/submissions/me", auth, h.GetMySubmissions)
}
