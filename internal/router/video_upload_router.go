package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupVideoUploadRoutes - Setup các routes cho video upload API
// Base path: /api/videos
// Tất cả routes đều cần authentication (trừ /health)
func SetupVideoUploadRoutes(api fiber.Router, videoHandler *handler.VideoUploadHandler, cfg *config.Config, redis *redis.Client) {
	// Video upload route group
	videoGroup := api.Group("/videos")

	// Health check endpoint (không cần authentication)
	videoGroup.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "Video upload service is running",
		})
	})

	// Thêm authentication middleware cho tất cả routes bên dưới
	authMiddleware := middleware.AuthMiddleware(cfg, redis)
	videoGroup.Use(authMiddleware)

	uploadGroup := videoGroup.Group("/upload")

	// POST /api/videos/upload/init - Khởi tạo upload session
	uploadGroup.Post("/init", videoHandler.InitVideoUpload)

	// POST /api/videos/upload/presigned-urls - Lấy presigned URLs cho chunks
	uploadGroup.Post("/presigned-urls", videoHandler.GetPresignedURLs)

	// POST /api/videos/upload/chunk-complete - Đánh dấu chunk hoàn thành
	uploadGroup.Post("/chunk-complete", videoHandler.CompleteChunkUpload)

	// POST /api/videos/upload/complete - Hoàn tất upload và bắt đầu processing
	uploadGroup.Post("/complete", videoHandler.CompleteVideoUpload)

	// GET /api/videos/upload/:upload_id/status - Lấy trạng thái upload
	uploadGroup.Get("/:upload_id/status", videoHandler.GetUploadStatus)

	// GET /api/videos/upload/:upload_id/resume - Lấy thông tin để resume upload
	uploadGroup.Get("/:upload_id/resume", videoHandler.GetResumeInfo)

	// GET /api/videos/upload/incomplete - Lấy danh sách uploads chưa hoàn thành
	uploadGroup.Get("/incomplete", videoHandler.GetIncompleteUploads)

	// DELETE /api/videos/upload/:upload_id - Hủy upload
	uploadGroup.Delete("/:upload_id", videoHandler.AbortUpload)

	// POST /api/videos/upload/:upload_id/reprocess - Re-enqueue video for processing
	uploadGroup.Post("/:upload_id/reprocess", videoHandler.ReprocessVideo)

	processingGroup := videoGroup.Group("/processing")

	processingGroup.Get("/queue", videoHandler.GetProcessingQueue)
}

func SetupHLSRoutes(api fiber.Router, hlsHandler *handler.HLSHandler) {
	// HLS streaming routes (public access)
	hlsGroup := api.Group("/hls")

	// Get video info and available qualities
	hlsGroup.Get("/:upload_id/info", hlsHandler.GetVideoInfo)

	// Get master playlist
	hlsGroup.Get("/:upload_id/master.m3u8", hlsHandler.GetMasterPlaylist)

	// Stream original video (fallback when HLS not ready)
	hlsGroup.Get("/:upload_id/video.mp4", hlsHandler.StreamOriginalVideo)

	// Get quality-specific playlist (e.g., /api/hls/{id}/v0/index.m3u8)
	hlsGroup.Get("/:upload_id/:quality/index.m3u8", hlsHandler.GetPlaylist)

	// Get video segments (e.g., /api/hls/{id}/v0/seg_00001.ts)
	hlsGroup.Get("/:upload_id/:quality/:segment", hlsHandler.GetSegment)
}
