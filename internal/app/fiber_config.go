package app

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/config"
)

// BuildFiberConfig (review PR #69 BLOCKER, QA 260927): trước bản vá này `fiber.New()` được gọi
// KHÔNG có config nào — `fiber.Ctx.IP()` (dùng bởi mọi rate limiter theo IP:
// AuthRateLimiter/WideAuthRateLimiter/OTPRateLimiter) khi đó LUÔN trả về TCP peer trực tiếp
// (`fasthttp.RemoteIP()`), không bao giờ đọc X-Forwarded-For dù header đó có mặt.
//
// Web gọi backend qua proxy server-side của Next.js
// (web/src/app/api/[...path]/route.ts#proxyRequest) — trình duyệt luôn nối tới Next.js trước,
// Next.js server (không phải trình duyệt) mới là bên THẬT SỰ mở kết nối TCP tới backend. Kết
// quả: MỌI request từ MỌI người dùng thật đều đứng chung 1 TCP peer ở tầng Fiber (IP của tiến
// trình Next.js) → mọi rate limiter theo IP chia sẻ DUY NHẤT 1 bucket cho toàn bộ website, không
// phải "mạng dùng chung IP" như PR S-P1-2 ban đầu mô tả — nghiêm trọng hơn nhiều (2-3 người dùng
// login gần như đồng thời có thể làm MỌI người khác bị 429).
//
// Cấu hình dưới đây tách riêng 2 việc: (1) DÙNG X-Forwarded-For khi có (ProxyHeader), (2) CHỈ tin
// header đó khi TCP peer thật sự nằm trong TrustedProxies (EnableTrustedProxyCheck) — nếu không,
// bất kỳ client nào cũng tự xưng IP giả qua header để né rate-limit hoặc mạo danh IP người khác.
// EnableIPValidation bật để Fiber trả về ĐÚNG 1 IP hợp lệ (địa chỉ ĐẦU TIÊN trong chuỗi
// X-Forwarded-For — theo quy ước, đó là IP client thật, các hop proxy nối thêm vào SAU), không
// phải nguyên chuỗi header thô (xem fiber@v2.52.12 ctx.go#extractIPFromHeader — hành vi thật đã
// đọc trực tiếp mã nguồn, không suy đoán).
func BuildFiberConfig(cfg *config.Config) fiber.Config {
	return fiber.Config{
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		EnableIPValidation:      true,
		TrustedProxies:          cfg.ResolvedTrustedProxies(),
	}
}
