package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupCategoryRoutes(
	api fiber.Router,
	cfg *config.Config,
	categoryHandler *handler.CategoryHandler,
	tagHandler *handler.TagHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// Categories - public read, auth required for write
	categories := api.Group("/categories")
	{
		categories.Get("/", categoryHandler.GetAllCategories)
		categories.Get("/:id", categoryHandler.GetCategoryByID)

		// Protected routes - require authentication
		categories.Post("/", auth, categoryHandler.CreateCategory)
		categories.Put("/:id", auth, categoryHandler.UpdateCategory)
		categories.Delete("/:id", auth, categoryHandler.DeleteCategory)
	}

	// Tags - public read, auth required for write
	tags := api.Group("/tags")
	{
		tags.Get("/", tagHandler.GetAllTags)
		tags.Get("/:id", tagHandler.GetTagByID)

		// Protected routes - require authentication
		tags.Post("/", auth, tagHandler.CreateTag)
		tags.Put("/:id", auth, tagHandler.UpdateTag)
		tags.Delete("/:id", auth, tagHandler.DeleteTag)
	}
}
