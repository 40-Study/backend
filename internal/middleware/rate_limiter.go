package middleware

import (
	"context"
	"fmt"
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
		config.KeyGenerator = func(c *fiber.Ctx) string {
			return c.IP()
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
			// If Redis fails, allow request but log error
			return c.Next()
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

		return c.Next()
	}
}

// AuthRateLimiter - Strict rate limiting for auth endpoints
// 5 attempts per minute per IP for login/register
func AuthRateLimiter(rdb *redis.Client) fiber.Handler {
	return RateLimiter(rdb, RateLimitConfig{
		Max:       5,
		Window:    time.Minute,
		KeyPrefix: "rate:auth",
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		Message: "Too many authentication attempts. Please wait before trying again.",
	})
}

// OTPRateLimiter - Rate limiting for OTP requests
// 3 OTP requests per 5 minutes per email
func OTPRateLimiter(rdb *redis.Client) fiber.Handler {
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
			return c.IP()
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
