package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupLessonContentRoutes(
	api fiber.Router,
	cfg *config.Config,
	lessonContentHandler *handler.LessonContentHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	lessons := api.Group("/lessons/:lessonId")
	{
		// Public read access
		lessons.Get("/video", lessonContentHandler.GetVideo)
		lessons.Get("/article", lessonContentHandler.GetArticle)
		lessons.Get("/attachments", lessonContentHandler.GetAttachments)

		// Protected write access
		lessons.Post("/video", auth, lessonContentHandler.CreateVideo)
		lessons.Put("/video", auth, lessonContentHandler.UpdateVideo)
		lessons.Delete("/video", auth, lessonContentHandler.DeleteVideo)

		lessons.Post("/article", auth, lessonContentHandler.CreateArticle)
		lessons.Put("/article", auth, lessonContentHandler.UpdateArticle)
		lessons.Delete("/article", auth, lessonContentHandler.DeleteArticle)

		lessons.Post("/attachments", auth, lessonContentHandler.CreateAttachment)
		lessons.Delete("/attachments/:id", auth, lessonContentHandler.DeleteAttachment)
	}
}
