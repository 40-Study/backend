package middleware

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/constants"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	// Max requests allowed in the window
	Max int
	// Time window duration
	Window time.Duration
	// Key prefix for Redis
	KeyPrefix string
	// Function to generate unique key (e.g., by IP, by email)
	KeyGenerator func(c *fiber.Ctx) string
	// Custom error message
	Message string
	// Skip rate limiting for certain conditions
	Skip func(c *fiber.Ctx) bool
	// Refund (nil = không bao giờ hoàn): chạy SAU handler; trả true thì lượt vừa tính được hoàn lại (DECR), tức request
	// đó không tiêu hạn mức. Dùng cho hành động tốn hạn mức chỉ khi THỰC SỰ có tác dụng (vd broadcast gửi tới >= 1
	// người): lỗi validate/không có người nhận không được khoá quản trị viên. Vẫn INCR trước handler nên hạn mức
	// nguyên tử với request song song; chỉ request bị chặn 429 hoặc Skip mới không bao giờ chạm tới Refund.
	Refund func(c *fiber.Ctx) bool
	// TrustedProxies (review vòng 2, PR #69): dùng bởi KeyGenerator mặc định (khi không set
	// riêng) để suy ra IP client qua ClientIP() thay vì c.IP() mặc định của Fiber — xem
	// ClientIPFromXFF (client_ip.go) cho lý do đầy đủ. nil/rỗng -> không IP nào được coi là
	// proxy đáng tin, mọi request dùng thẳng TCP peer, bỏ qua X-Forwarded-For hoàn toàn.
	TrustedProxies *TrustedProxySet
}

// RateLimiter creates a rate limiting middleware using Redis
func RateLimiter(rdb *redis.Client, config RateLimitConfig) fiber.Handler {
	// Set defaults
	if config.Max == 0 {
		config.Max = 10
	}
	if config.Window == 0 {
		config.Window = time.Minute
	}
	if config.KeyPrefix == "" {
		config.KeyPrefix = "rate_limit"
	}
	if config.KeyGenerator == nil {
		trusted := config.TrustedProxies
		config.KeyGenerator = func(c *fiber.Ctx) string {
			return ClientIP(c, trusted)
		}
	}
	if config.Message == "" {
		config.Message = "Too many requests, please try again later"
	}

	return func(c *fiber.Ctx) error {
		// Check skip condition
		if config.Skip != nil && config.Skip(c) {
			return c.Next()
		}

		ctx := c.Context()
		key := fmt.Sprintf("%s:%s", config.KeyPrefix, config.KeyGenerator(c))

		// Increment counter
		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			// M-02 (audit 260909): trước đây fail-open (cho qua khi Redis lỗi) — Redis down
			// đồng nghĩa brute-force login/OTP không còn giới hạn. Mọi route dùng RateLimiter()
			// đều fail-closed (503) khi Redis lỗi: AuthRateLimiter/OTPRateLimiter (đăng nhập/OTP/
			// reset-password/select-role/refresh-token) và giới hạn theo user của cuộc thi
			// (join/start/submit, router/contest_routes.go) — Redis down thì các route này tạm 503.
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Rate limiting service unavailable, please try again later",
			})
		}

		// Set expiry on first request
		if count == 1 {
			rdb.Expire(ctx, key, config.Window)
		}

		// Get TTL for Retry-After header
		ttl, _ := rdb.TTL(ctx, key).Result()

		// Set rate limit headers
		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", config.Max))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, config.Max-int(count))))
		c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(ttl).Unix()))

		// Check if limit exceeded
		if int(count) > config.Max {
			c.Set("Retry-After", fmt.Sprintf("%d", int(ttl.Seconds())))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       config.Message,
				"retry_after": int(ttl.Seconds()),
			})
		}

		if config.Refund == nil {
			return c.Next()
		}
		nextErr := c.Next()
		if config.Refund(c) {
			refundRateLimitSlot(ctx, rdb, key)
		}
		return nextErr
	}
}

// refundRateLimitSlot trả lại MỘT lượt vừa INCR (xem RateLimitConfig.Refund). Nếu key đã hết hạn giữa INCR và DECR,
// DECR tạo ra bộ đếm âm KHÔNG có TTL — nó không bao giờ tự xoá và sẽ làm lệch mọi cửa sổ sau, nên xoá ngay. Lỗi Redis
// ở đây chỉ log: lượt đã tiêu giữ nguyên (nghiêng về phía chặn, an toàn cho hành động không hoàn tác được).
func refundRateLimitSlot(ctx context.Context, rdb *redis.Client, key string) {
	n, err := rdb.Decr(ctx, key).Result()
	if err != nil {
		log.Printf("[RateLimiter] không hoàn được lượt cho %s: %v", key, err)
		return
	}
	if n < 0 {
		if err := rdb.Del(ctx, key).Err(); err != nil {
			log.Printf("[RateLimiter] không xoá được bộ đếm âm %s: %v", key, err)
		}
	}
}

// AuthRateLimiter - Strict rate limiting for auth endpoints
// 5 attempts per minute per IP. CHỈ dành cho bề mặt đoán mật khẩu/OTP thật sự:
// /login, /register, /reset-password (nhận + xác nhận OTP/mật khẩu mới). KHÔNG dùng cho bất kỳ
// route nào chỉ chạy được SAU KHI đã có credential/pending-token/access-token hợp lệ từ một bước
// trước đó (select-role, select-org, refresh-token) — xem WideAuthRateLimiter bên dưới, và
// S-P1-2 (QA 260927) cho lý do đầy đủ.
func AuthRateLimiter(rdb *redis.Client, trusted *TrustedProxySet) fiber.Handler {
	return RateLimiter(rdb, RateLimitConfig{
		Max:       5,
		Window:    time.Minute,
		KeyPrefix: "rate:auth",
		KeyGenerator: func(c *fiber.Ctx) string {
			return ClientIP(c, trusted)
		},
		Message: "Too many authentication attempts. Please wait before trying again.",
	})
}

// WideAuthRateLimiter (S-P1-2, QA 260927 — bổ sung theo phản hồi coordinator sau khi PR #69 mở):
// limiter DÙNG CHUNG, cấu hình được keyPrefix/max, cho các endpoint auth chỉ chạy được SAU KHI
// đã có credential/pending-token/access-token hợp lệ từ một bước trước đó — không phải bề mặt dò
// mật khẩu/OTP như /login. Tổng quát hoá thay vì viết thêm một hàm gần giống AuthRateLimiter cho
// mỗi route thuộc nhóm này (ban đầu chỉ có refresh-token; giờ thêm select-role/select-org).
//
// Lý do tách khỏi AuthRateLimiter: /select-role và /select-org (M-03/N5, audit 260909) TRƯỚC ĐÂY
// dùng chung bucket "rate:auth" (5/phút/IP) với /login — nhưng MỘT lần đăng nhập trọn vẹn đã là
// login + select-role (+ select-org nếu đổi tổ chức), tức tiêu 2-3 lượt trong CÙNG 1 bucket 5
// lượt/phút. Một mạng dùng chung IP (trường học, văn phòng) vì vậy chỉ đăng nhập trọn được
// khoảng 2 lần/phút thay vì 5 — RẤT dễ hiểu nhầm là bug đăng nhập trong khi thực ra là rate-limit
// tự-chặn-chính-mình. /select-role/select-org không phải bề mặt dò mật khẩu (đã qua bước xác thực
// mật khẩu ở /login, hoặc đã cầm access token thật) nên xứng đáng một ngưỡng rộng hơn nhiều,
// tương tự lý do refresh-token đã tách trước đó.
func WideAuthRateLimiter(rdb *redis.Client, keyPrefix string, max int, trusted *TrustedProxySet) fiber.Handler {
	return RateLimiter(rdb, RateLimitConfig{
		Max:       max,
		Window:    time.Minute,
		KeyPrefix: keyPrefix,
		KeyGenerator: func(c *fiber.Ctx) string {
			return ClientIP(c, trusted)
		},
		Message: "Too many requests. Please wait before trying again.",
	})
}

// OTPRateLimiter - Rate limiting for OTP requests
// 3 OTP requests per 5 minutes per email
func OTPRateLimiter(rdb *redis.Client, trusted *TrustedProxySet) fiber.Handler {
	return RateLimiter(rdb, RateLimitConfig{
		Max:       3,
		Window:    5 * time.Minute,
		KeyPrefix: "rate:otp",
		KeyGenerator: func(c *fiber.Ctx) string {
			// Try to get email from body
			var body struct {
				Email string `json:"email"`
			}
			if err := c.BodyParser(&body); err == nil && body.Email != "" {
				return body.Email
			}
			return ClientIP(c, trusted)
		},
		Message: "Too many OTP requests. Please wait 5 minutes before requesting another code.",
	})
}

// LoginAttemptTracker tracks failed login attempts and locks accounts
type LoginAttemptTracker struct {
	rdb             *redis.Client
	maxAttempts     int
	lockoutDuration time.Duration
	windowDuration  time.Duration
}

func NewLoginAttemptTracker(rdb *redis.Client) *LoginAttemptTracker {
	return &LoginAttemptTracker{
		rdb:             rdb,
		maxAttempts:     5,
		lockoutDuration: 15 * time.Minute,
		windowDuration:  5 * time.Minute,
	}
}

// RecordFailedAttempt records a failed login attempt
// Returns true if account should be locked
func (t *LoginAttemptTracker) RecordFailedAttempt(ctx context.Context, email string) (bool, int, error) {
	key := constants.KeyLoginAttempts(email)

	count, err := t.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, 0, err
	}

	// Set expiry on first attempt
	if count == 1 {
		t.rdb.Expire(ctx, key, t.windowDuration)
	}

	remaining := t.maxAttempts - int(count)
	if remaining < 0 {
		remaining = 0
	}

	// Lock account if max attempts exceeded
	if int(count) >= t.maxAttempts {
		lockKey := constants.KeyLoginLocked(email)
		t.rdb.Set(ctx, lockKey, "1", t.lockoutDuration)
		return true, remaining, nil
	}

	return false, remaining, nil
}

// IsLocked checks if account is locked
func (t *LoginAttemptTracker) IsLocked(ctx context.Context, email string) (bool, time.Duration, error) {
	lockKey := constants.KeyLoginLocked(email)

	exists, err := t.rdb.Exists(ctx, lockKey).Result()
	if err != nil {
		return false, 0, err
	}

	if exists == 0 {
		return false, 0, nil
	}

	ttl, err := t.rdb.TTL(ctx, lockKey).Result()
	if err != nil {
		return true, t.lockoutDuration, nil
	}

	return true, ttl, nil
}

// ClearAttempts clears failed attempts after successful login
func (t *LoginAttemptTracker) ClearAttempts(ctx context.Context, email string) error {
	key := constants.KeyLoginAttempts(email)
	return t.rdb.Del(ctx, key).Err()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
