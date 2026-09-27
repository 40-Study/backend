package middleware

// Review vòng 3 (plans/reports/review-260928-round3-pr69-pr25.md) — BLOCKER có điều kiện: nếu
// cổng 3000/5000 của backend từng lộ trực tiếp (không qua nginx — cấu hình sai, firewall hỏng,
// tunnel debug, ...), client có thể tự gửi X-Forwarded-For; Next.js `??=` GIỮ NGUYÊN giá trị đó
// (không ghi đè) vì header đã tồn tại từ trước khi tới Next — client xoay XFF liên tục để mỗi
// request rơi vào 1 "IP" khác nhau, né hoàn toàn rate-limit theo IP dù ClientIP() (client_ip.go)
// đã đúng thuật toán duyệt-từ-phải. Đây là phòng thủ CHIỀU SÂU (defense in depth): giới hạn
// theo TÀI KHOẢN (khoá email/OTP-target), hoàn toàn không phụ thuộc IP/XFF — vẫn chặn được kể cả
// khi lớp rate-limit theo IP bị né hoàn toàn.
//
// Áp dụng cho MỌI endpoint mà kẻ tấn công dò một bí mật gắn với 1 tài khoản cụ thể: /login (dò
// mật khẩu), /register (dò mã OTP đăng ký), /reset-password (dò mã OTP đặt lại mật khẩu). KHÔNG
// áp cho /reset-password/request hay /register/request (bước GỬI OTP) — hai route đó đã dùng
// OTPRateLimiter, vốn cũng khoá theo email (không phải IP) với ngưỡng chặt hơn (3 lần/5 phút).

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// Ngưỡng mặc định — hằng có tên, không phải số ma thuật rải rác (theo đúng yêu cầu review vòng
// 3): 10 lần thất bại (sai mật khẩu / sai OTP / bất kỳ response không phải 2xx nào khác) trong
// 15 phút thì khoá tài khoản đó thêm 15 phút. Callable riêng cho từng route qua
// AccountLockoutConfig nếu cần ngưỡng khác nhau (hiện chưa cần).
const (
	DefaultAccountLockoutMaxFailures     = 10
	DefaultAccountLockoutWindow          = 15 * time.Minute
	DefaultAccountLockoutLockoutDuration = 15 * time.Minute
	DefaultAccountLockoutMessage         = "Too many failed attempts for this account. Please wait before trying again."
)

// AccountLockoutConfig cấu hình 1 instance AccountFailureLockout.
type AccountLockoutConfig struct {
	// KeyPrefix namespaces Redis key của route này (mỗi route gọi AccountFailureLockout dùng
	// prefix riêng để không đụng bucket của route khác, dù cùng email).
	KeyPrefix string
	// MaxFailures/Window/LockoutDuration: 0 = dùng giá trị mặc định ở trên.
	MaxFailures     int
	Window          time.Duration
	LockoutDuration time.Duration
	// Message trả trong response 429 khi tài khoản đang bị khoá. Để trống = dùng
	// DefaultAccountLockoutMessage — CHỦ Ý dùng đúng 1 câu chung cho MỌI email (tồn tại hay
	// không), không phân nhánh theo email để tránh lộ email nào có tồn tại trong hệ thống.
	Message string
}

// AccountFailureLockout (review vòng 3, PR #69): khoá tạm 1 email sau MaxFailures lần request
// KHÔNG thành công (bất kỳ status không phải 2xx nào — sai mật khẩu, sai OTP, request không hợp
// lệ đều tính) trong vòng Window; đăng nhập/xác thực đúng (status 2xx) reset bộ đếm về 0.
//
// Độc lập HOÀN TOÀN với IP/X-Forwarded-For — key Redis chỉ dựa vào email (chuẩn hoá
// lowercase+trim) lấy từ JSON body, nên xoay IP/XFF liên tục không né được giới hạn này (khác
// với AuthRateLimiter/WideAuthRateLimiter, vốn khoá theo IP suy ra từ ClientIP()).
//
// Response 429 dùng CÙNG shape với RateLimiter() (`{"error", "retry_after"}` + header
// `Retry-After`) để web (#25) không cần đổi gì — `api-client.ts` chỉ phân biệt `RateLimitError`
// qua HTTP status 429, không đọc field nào trong body.
//
// Không thể xác định email từ body (JSON hỏng, thiếu field "email") -> bỏ qua, để middleware/
// handler phía sau tự xử lý lỗi validate — route này không phải nơi duy nhất bảo vệ endpoint
// (AuthRateLimiter theo IP vẫn chạy song song).
func AccountFailureLockout(rdb *redis.Client, cfg AccountLockoutConfig) fiber.Handler {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = DefaultAccountLockoutMaxFailures
	}
	if cfg.Window <= 0 {
		cfg.Window = DefaultAccountLockoutWindow
	}
	if cfg.LockoutDuration <= 0 {
		cfg.LockoutDuration = DefaultAccountLockoutLockoutDuration
	}
	if cfg.Message == "" {
		cfg.Message = DefaultAccountLockoutMessage
	}

	return func(c *fiber.Ctx) error {
		email := normalizeEmailFromJSONBody(c)
		if email == "" {
			return c.Next()
		}

		ctx := c.Context()
		lockKey := cfg.KeyPrefix + ":lock:" + email
		failKey := cfg.KeyPrefix + ":fail:" + email

		if ttl, err := rdb.TTL(ctx, lockKey).Result(); err == nil && ttl > 0 {
			c.Set("Retry-After", fmt.Sprintf("%d", int(ttl.Seconds())))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       cfg.Message,
				"retry_after": int(ttl.Seconds()),
			})
		}

		handlerErr := c.Next()

		status := c.Response().StatusCode()
		if status >= 200 && status < 300 {
			rdb.Del(ctx, failKey)
			return handlerErr
		}

		count, err := rdb.Incr(ctx, failKey).Result()
		if err != nil {
			// Redis lỗi ở bước ĐẾM (không phải bước chặn) — không fail-closed cả request gốc vì
			// handler phía sau đã chạy xong và trả response hợp lệ rồi; chỉ log bằng cách để
			// nguyên request đó không được đếm (an toàn hơn so với chặn nhầm request đã xử lý).
			return handlerErr
		}
		if count == 1 {
			rdb.Expire(ctx, failKey, cfg.Window)
		}
		if int(count) >= cfg.MaxFailures {
			rdb.Set(ctx, lockKey, "1", cfg.LockoutDuration)
		}

		return handlerErr
	}
}

// normalizeEmailFromJSONBody đọc field "email" từ JSON body (dùng chung cho LoginRequestDto,
// VerifyOtpRequestDto, ResetPasswordRequestDto — cả 3 đều dùng đúng tên field JSON "email"),
// chuẩn hoá lowercase + trim để "User@Test.com" và " user@test.com " cùng rơi vào 1 bucket.
// KHÔNG dùng c.BodyParser vào DTO thật của route (tránh phụ thuộc ngược vào package dto) — chỉ
// cần đúng 1 field, và BodyParser đọc lại được nhiều lần trên cùng request (fasthttp giữ body
// dạng []byte tĩnh, không phải stream) nên không ảnh hưởng BodyParser thật của handler phía sau.
func normalizeEmailFromJSONBody(c *fiber.Ctx) string {
	var body struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&body); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(body.Email))
}
