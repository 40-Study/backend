package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupNoteRoutes — ghi chú theo mốc thời gian trong bài học (Phase 1 §3). Toàn bộ route yêu
// cầu auth: ghi chú luôn gắn với MỘT người dùng cụ thể, không có nhánh công khai.
func SetupNoteRoutes(api fiber.Router, cfg *config.Config, h *handler.NoteHandler, redis *redis.Client) {
	auth := middleware.AuthMiddleware(cfg, redis)

	api.Get("/lessons/:lessonId/notes", auth, h.ListByLesson)
	api.Post("/lessons/:lessonId/notes", auth, h.CreateNote)
	api.Get("/courses/:courseId/notes", auth, h.ListByCourse)
	api.Put("/notes/:id", auth, h.UpdateNote)
	api.Delete("/notes/:id", auth, h.DeleteNote)
}
