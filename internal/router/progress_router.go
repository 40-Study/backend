package router

// TODO: Implement ProgressHandler first, then uncomment this router

// import (
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/redis/go-redis/v9"
// 	"study.com/v1/internal/config"
// 	"study.com/v1/internal/handler"
// 	"study.com/v1/internal/middleware"
// )

// func SetupProgressRoutes(
// 	api fiber.Router,
// 	cfg *config.Config,
// 	progressHandler *handler.ProgressHandler,
// 	redis *redis.Client,
// ) {
// 	auth := middleware.AuthMiddleware(cfg, redis)

// 	// ============================================================================
// 	// LESSON PROGRESS - Tien do bai hoc
// 	// ============================================================================
// 	// POST   /api/lessons/:lessonId/track-time         - Track thoi gian hoc
// 	// POST   /api/lessons/:lessonId/track-position     - Track vi tri video
// 	// POST   /api/lessons/:lessonId/complete           - Danh dau hoan thanh
// 	// GET    /api/lessons/:lessonId/my-progress        - Lay tien do cua toi

// 	api.Post("/lessons/:lessonId/track-time", auth, progressHandler.TrackTime)
// 	api.Post("/lessons/:lessonId/track-position", auth, progressHandler.TrackPosition)
// 	api.Post("/lessons/:lessonId/complete", auth, progressHandler.MarkComplete)
// 	api.Get("/lessons/:lessonId/my-progress", auth, progressHandler.GetMyLessonProgress)

// 	// ============================================================================
// 	// COURSE PROGRESS - Tien do khoa hoc
// 	// ============================================================================
// 	// GET    /api/courses/:courseId/my-progress        - Tien do khoa hoc cua toi
// 	// GET    /api/courses/:courseId/progress-detail    - Chi tiet tien do (tat ca lessons)

// 	api.Get("/courses/:courseId/my-progress", auth, progressHandler.GetMyCourseProgress)
// 	api.Get("/courses/:courseId/progress-detail", auth, progressHandler.GetCourseProgressDetail)

// 	// ============================================================================
// 	// BULK TRACKING - Batch update
// 	// ============================================================================
// 	// POST   /api/progress/batch                       - Batch update progress (offline sync)

// 	api.Post("/progress/batch", auth, progressHandler.BatchUpdateProgress)
// }
