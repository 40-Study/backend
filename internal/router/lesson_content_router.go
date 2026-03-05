package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupLessonContentRoutes(
	api fiber.Router,
	lessonContentHandler *handler.LessonContentHandler,
) {
	lessons := api.Group("/lessons/:lessonId")
	{
		// Video (1:1)
		lessons.Post("/video", lessonContentHandler.CreateVideo)
		lessons.Get("/video", lessonContentHandler.GetVideo)
		lessons.Put("/video", lessonContentHandler.UpdateVideo)
		lessons.Delete("/video", lessonContentHandler.DeleteVideo)

		// Article (1:1)
		lessons.Post("/article", lessonContentHandler.CreateArticle)
		lessons.Get("/article", lessonContentHandler.GetArticle)
		lessons.Put("/article", lessonContentHandler.UpdateArticle)
		lessons.Delete("/article", lessonContentHandler.DeleteArticle)

		// Attachments (1:many)
		lessons.Post("/attachments", lessonContentHandler.CreateAttachment)
		lessons.Get("/attachments", lessonContentHandler.GetAttachments)
		lessons.Delete("/attachments/:id", lessonContentHandler.DeleteAttachment)
	}
}
