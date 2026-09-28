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
	permChecker *middleware.PermissionChecker,
) {
	auth := middleware.AuthMiddleware(cfg, redis)
	// A-P0-2 (QA 260927): trước đây các route ghi chỉ có `auth` — bất kỳ user đã đăng nhập nào
	// (kể cả STUDENT) cũng sửa/xoá được danh mục và tag gốc của toàn hệ thống. CATEGORIES_SYSTEM_
	// MANAGE chỉ SYSTEM_ADMIN có (qua wildcard "*"), khớp đúng yêu cầu "category/tag CRUD chỉ admin".
	requireSystemManage := permChecker.RequirePermissions("CATEGORIES_SYSTEM_MANAGE")

	// Categories - public read, admin-only write
	categories := api.Group("/categories")
	{
		categories.Get("/", categoryHandler.GetAllCategories)
		categories.Get("/:id", categoryHandler.GetCategoryByID)

		// Protected routes - require authentication + CATEGORIES_SYSTEM_MANAGE
		categories.Post("/", auth, requireSystemManage, categoryHandler.CreateCategory)
		categories.Put("/:id", auth, requireSystemManage, categoryHandler.UpdateCategory)
		categories.Delete("/:id", auth, requireSystemManage, categoryHandler.DeleteCategory)
	}

	// Tags - public read, admin-only write
	tags := api.Group("/tags")
	{
		tags.Get("/", tagHandler.GetAllTags)
		tags.Get("/:id", tagHandler.GetTagByID)

		// Protected routes - require authentication + CATEGORIES_SYSTEM_MANAGE
		tags.Post("/", auth, requireSystemManage, tagHandler.CreateTag)
		tags.Put("/:id", auth, requireSystemManage, tagHandler.UpdateTag)
		tags.Delete("/:id", auth, requireSystemManage, tagHandler.DeleteTag)
	}
}
