package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

// SetupPublicTeacherProfileRoutes (PG-M2) - GET /teachers/:id/public-profile, công khai (không AuthMiddleware):
// chỉ trả dữ liệu đã an toàn để công khai (xem dto.PublicTeacherPageDTO). Đăng ký riêng ở app.go như wishlist.
func SetupPublicTeacherProfileRoutes(api fiber.Router, h *handler.PublicTeacherProfileHandler) {
	api.Get("/teachers/:id/public-profile", h.GetPublicProfile)
}
