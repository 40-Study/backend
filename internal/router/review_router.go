package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupReviewRoutes(
	api fiber.Router,
	cfg *config.Config,
	reviewHandler *handler.ReviewHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// ============================================================================
	// COURSE REVIEWS
	// ============================================================================
	// Public: get reviews
	api.Get("/courses/:courseId/reviews", reviewHandler.GetReviewsByCourse)

	// Auth required: create/update/delete
	api.Post("/courses/:courseId/reviews", auth, reviewHandler.CreateReview)

	reviews := api.Group("/reviews", auth)
	{
		reviews.Put("/:id", reviewHandler.UpdateReview)
		reviews.Delete("/:id", reviewHandler.DeleteReview)
		reviews.Post("/:id/reaction", reviewHandler.ReactToReview)
		reviews.Delete("/:id/reaction", reviewHandler.RemoveReaction)
	}
}
