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
		// H-03: từ chối refresh token bị dùng làm Bearer access token.
		if claims.TokenType != utils.TokenTypeAccess {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token type",
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
			// Phase 1 quản lý người dùng: user_version cũng bump khi đổi mật khẩu/đăng xuất
			// nơi khác, nên KHÔNG thể suy ra "bị khoá" chỉ từ việc user_version lệch — kiểm
			// thêm marker riêng (chỉ 1 lần GET Redis, trên nhánh lỗi hiếm gặp, KHÔNG phải mỗi
			// request bình thường) để trả thông báo đúng lý do cho FE thay vì "All sessions
			// revoked" chung chung cho mọi trường hợp.
			if locked, _ := rdb.Exists(c.Context(), constants.KeyAccountLocked(claims.UserID.String())).Result(); locked > 0 {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"message": "Tài khoản đã bị khoá",
					"code":    "ACCOUNT_LOCKED",
					"error":   "Please login again",
				})
			}
			// Phase 3 (quyết định #6): lần bump gần nhất là admin ĐỔI VAI TRÒ (duyệt hồ sơ giáo
			// viên) -> báo code riêng; POST /auth/refresh-token sẽ cấp token mới với vai trò mới
			// (xem service/auth_service_role_refresh.go), web tự refresh không bắt đăng nhập lại.
			if marker, mErr := rdb.Get(c.Context(), constants.KeyRoleChanged(claims.UserID.String())).Result(); mErr == nil && marker == userVerStr {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"message": "Role changed",
					"code":    "ROLE_CHANGED",
					"error":   "Please refresh token",
				})
			}
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
