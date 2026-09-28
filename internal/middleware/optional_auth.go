package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
)

// OptionalAuth (MVP "Cuộc thi", contract §2.2) — cho route công khai nhưng trả dữ liệu khác nhau
// theo người xem (chi tiết cuộc thi, BXH):
//   - KHÔNG có token (cookie accessToken lẫn header Bearer) → đi tiếp như khách, không set Locals.
//   - CÓ token → xử lý y hệt AuthMiddleware, kể cả 401 khi token sai/hết hạn. Cố ý không lặng lẽ
//     hạ xuống "khách": web dựa vào 401 để tự refresh token rồi gọi lại, nếu trả dữ liệu của khách
//     thì người đã đăng nhập sẽ thấy nút "Đăng nhập để tham gia" sai sự thật.
func OptionalAuth(cfg *config.Config, rdb *redis.Client) fiber.Handler {
	auth := AuthMiddleware(cfg, rdb)
	return func(c *fiber.Ctx) error {
		if c.Cookies("accessToken") == "" && c.Get("Authorization") == "" {
			return c.Next()
		}
		return auth(c)
	}
}
