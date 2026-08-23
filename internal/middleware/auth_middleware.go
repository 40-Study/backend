package middleware

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/utils"
)

func AuthMiddleware(cfg *config.Config, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Ưu tiên cookie, fallback Authorization header
		accessToken := c.Cookies("accessToken")
		if accessToken == "" {
			authHeader := c.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				accessToken = authHeader[7:]
			}
		}

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

		// ===== 3. Check user_version (for logout all) =====
		userVersionKey := constants.KeyUserVersion(claims.UserID.String())
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
		c.Locals("active_role", claims.ActiveRole)
		if claims.ActiveOrgID != nil {
			c.Locals("active_org_id", *claims.ActiveOrgID)
		}

		return c.Next()
	}
}

func RequirePermissions(permissions ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return nil
	}
}

// OptionalAuthMiddleware extracts user info if token is present, but doesn't require it
func OptionalAuthMiddleware(cfg *config.Config, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Try to get token from cookie or header
		accessToken := c.Cookies("accessToken")
		if accessToken == "" {
			authHeader := c.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				accessToken = authHeader[7:]
			}
		}

		// No token - continue without user info
		if accessToken == "" {
			return c.Next()
		}

		// Try to parse token
		claims, err := utils.ParseToken(cfg, accessToken)
		if err != nil {
			// Invalid token - continue without user info
			return c.Next()
		}

		// Check user_version (for logout all)
		userVersionKey := constants.KeyUserVersion(claims.UserID.String())
		userVerStr, err := rdb.Get(c.Context(), userVersionKey).Result()
		if err != nil {
			// Redis error or session not found - continue without user info
			return c.Next()
		}

		userVersion, _ := strconv.ParseInt(userVerStr, 10, 64)
		if userVersion != claims.UserVersion {
			// Session revoked - continue without user info
			return c.Next()
		}

		// Set user info in context
		c.Locals("user_id", claims.UserID)
		c.Locals("device_id", claims.DeviceID)
		c.Locals("active_role", claims.ActiveRole)
		if claims.ActiveOrgID != nil {
			c.Locals("active_org_id", *claims.ActiveOrgID)
		}

		return c.Next()
	}
}
