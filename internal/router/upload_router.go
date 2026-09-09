package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupUploadRoutes(api fiber.Router, cfg *config.Config, uploadHandler *handler.UploadHandler, redis *redis.Client, permChecker *middleware.PermissionChecker) {
	// All upload routes require authentication
	upload := api.Group("/upload", middleware.AuthMiddleware(cfg, redis))

	// POST /api/upload - Upload image only
	upload.Post("/", uploadHandler.UploadImage)

	// POST /api/upload/any - Upload any file (image or video)
	upload.Post("/any", uploadHandler.Upload)

	// DELETE /api/upload?url=... - Delete file by URL
	// C-14 (audit 260909): không có bảng ownership file (uploader <-> object name) trong
	// codebase nên không thể xác định "ai được xóa file của chính mình" ở tầng service mà
	// không thêm model/migration mới (ngoài phạm vi sửa lần này). Theo hướng dẫn "nếu không
	// có convention prefix thì yêu cầu * permission" -> dùng SYSTEM_SETTINGS_MANAGE (permission
	// cụ thể chỉ SYSTEM_ADMIN nắm, theo data/permissions/system_admin_permissions.json) thay vì
	// literal "*": seeder mở rộng "*" của SYSTEM_ADMIN thành từng permission cụ thể khi ghi vào
	// system_role_permissions (xem seeds/seeder.go SeedRoles), nên không user nào có permission
	// tên đúng là "*" trong DB — RequirePermissions("*") sẽ khóa cả SYSTEM_ADMIN nếu dùng ở đây.
	// Đồng thời UploadService.DeleteByURL whitelist bucket để không xóa nhầm sang bucket khác.
	upload.Delete("/", permChecker.RequirePermissions("SYSTEM_SETTINGS_MANAGE"), uploadHandler.DeleteFile)
}
