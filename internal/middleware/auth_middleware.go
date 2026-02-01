package middleware

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/utils"
)

func AuthMiddleware(cfg *config.Config, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// ===== 1. Get access token from Header or Cookie =====
		var accessToken string

		// Try Header first: Authorization: Bearer <token>
		authHeader := c.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			accessToken = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// Fallback to Cookie
		if accessToken == "" {
			accessToken = c.Cookies("accessToken")
		}

		if accessToken == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing access token",
			})
		}

		// ===== 2. Parse and validate JWT =====
		claims, err := utils.ParseToken(cfg, accessToken)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		// ===== 3. Check user_version (for logout all) =====
		userVersionKey := fmt.Sprintf("user_version:%s", claims.UserID)
		userVerStr, err := rdb.Get(c.Context(), userVersionKey).Result()
		if err == redis.Nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "User session not found",
				"error":   "Please login again",
			})
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Redis connection error",
				"error":   err.Error(),
			})
		}

		userVersion, _ := strconv.ParseInt(userVerStr, 10, 64)
		if userVersion != claims.UserVersion {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "All sessions revoked",
				"error":   "Please login again",
			})
		}

		// ===== 4. Set user info in context =====
		c.Locals("user_id", claims.UserID)
		c.Locals("device_id", claims.DeviceID)

		return c.Next()
	}
}

// Hybrid approach:
// - Logout 1 device: Xóa refresh token từ Redis (access token vẫn valid đến hết hạn)
// - Logout all devices: Increment userVersion → TẤT CẢ tokens invalid ngay lập tức

func RequirePermissions(permissions ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return nil
	}
}
