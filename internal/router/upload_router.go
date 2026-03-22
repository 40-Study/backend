package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupUploadRoutes(api fiber.Router, cfg *config.Config, uploadHandler *handler.UploadHandler, redis *redis.Client) {
	// All upload routes require authentication
	upload := api.Group("/upload", middleware.AuthMiddleware(cfg, redis))

	// POST /api/upload - Upload image only
	upload.Post("/", uploadHandler.UploadImage)

	// POST /api/upload/any - Upload any file (image or video)
	upload.Post("/any", uploadHandler.Upload)

	// DELETE /api/upload?url=... - Delete file by URL
	upload.Delete("/", uploadHandler.DeleteFile)
}
