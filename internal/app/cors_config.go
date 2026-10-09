package app

import "github.com/gofiber/fiber/v2/middleware/cors"

// BuildCORSConfig dựng cấu hình CORS của API. Idempotency-Key (QA 261009, M2) phải nằm trong AllowHeaders: trình duyệt
// gọi chéo origin sẽ gửi preflight, thiếu header này thì preflight bị từ chối và request gửi broadcast không bao giờ
// tới server. Idempotent-Replay phải được expose để web đọc được cờ "đây là bản phát lại".
func BuildCORSConfig(allowedOrigins string) cors.Config {
	return cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS, PATCH",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, Idempotency-Key",
		ExposeHeaders:    "Idempotent-Replay",
		AllowCredentials: true,
	}
}
