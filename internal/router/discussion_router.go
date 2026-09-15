package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupDiscussionRoutes(api fiber.Router, cfg *config.Config, h *handler.DiscussionHandler, redis *redis.Client) {
	discussions := api.Group("/discussions")

	// Public routes
	discussions.Get("/", h.ListPosts)
	discussions.Get("/:slug", h.GetPostBySlug)

	// Auth-required routes
	auth := discussions.Group("")
	auth.Use(middleware.AuthMiddleware(cfg, redis))

	// Hỏi đáp theo bài học (Phase 1 §5, CAO-5 review vòng 2): route này TRƯỚC ĐÂY đăng ký thẳng
	// trên `api` (không auth), nên c.Locals("user_id") không bao giờ được điền — user_vote luôn
	// rỗng/sai với CHÍNH người đang đăng nhập, dù ListByLesson đã VIẾT SẴN logic đọc user_id
	// optional (xem discussion_handler.go). Chuyển vào nhóm auth theo đúng quyết định review:
	// route giờ yêu cầu đăng nhập (khác với dự định "công khai" ban đầu trong comment cũ), đổi
	// lại là user_vote tính đúng — nhất quán với mọi endpoint viết (vote/comment) vốn đã auth.
	api.Get("/lessons/:lessonId/discussions", middleware.AuthMiddleware(cfg, redis), h.ListByLesson)
	auth.Post("/", h.CreatePost)
	auth.Post("/:slug/comments", h.AddComment)
	auth.Post("/:id/vote", h.Vote)
	auth.Delete("/:id/vote", h.RemoveVote)
	auth.Delete("/:id", h.DeletePost)
}
