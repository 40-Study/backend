package middleware

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"tiger.com/v2/src/config"
	"tiger.com/v2/src/utils"
)

func AuthMiddleware(cfg *config.Config, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {

		accessToken := c.Cookies("accessToken")
		if accessToken == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing access token",
			})
		}

		claims, err := utils.ParseToken(cfg, accessToken)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		key := fmt.Sprintf("session_version:%s", claims.UserID)

		versionStr, err := rdb.Get(c.Context(), key).Result()
		if err == redis.Nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "Session not found or expired",
				"error":   "Please login again",
			})
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Redis connection error",
				"error":   err.Error(),
			})
		}

		versionInt, err := strconv.ParseInt(versionStr, 10, 64)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Invalid session version format",
				"error":   err.Error(),
			})
		}

		if versionInt != claims.Version {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "Session revoked",
				"error":   "Please login again",
			})
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("device_id", claims.DeviceID)
		c.Locals("version", claims.Version)

		return c.Next()
	}
}

func RequirePermissions(permissions ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return nil
	}
}
