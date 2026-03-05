package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupCategoryRoutes(
	api fiber.Router,
	categoryHandler *handler.CategoryHandler,
	tagHandler *handler.TagHandler,
) {
	// Categories
	categories := api.Group("/categories")
	{
		categories.Post("/", categoryHandler.CreateCategory)
		categories.Get("/", categoryHandler.GetAllCategories)
		categories.Get("/:id", categoryHandler.GetCategoryByID)
		categories.Put("/:id", categoryHandler.UpdateCategory)
		categories.Delete("/:id", categoryHandler.DeleteCategory)
	}

	// Tags
	tags := api.Group("/tags")
	{
		tags.Post("/", tagHandler.CreateTag)
		tags.Get("/", tagHandler.GetAllTags)
		tags.Delete("/:id", tagHandler.DeleteTag)
	}
}
