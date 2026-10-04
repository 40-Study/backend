package middleware

import "github.com/gofiber/fiber/v2"

// DenyActiveRole chặn (403) request khi vai trò đang hoạt động trong token (c.Locals("active_role"), do
// AuthMiddleware gắn) nằm trong `roles`. Dùng cho khu mà một vai không dùng theo chính sách sản phẩm, ví dụ
// phụ huynh không dùng ví xu. Chặn theo vai ĐANG CHỌN, không theo mọi vai người đó giữ: tài khoản vừa là học viên
// vừa là phụ huynh vẫn dùng được ví khi chọn vai học viên. Phải đặt SAU AuthMiddleware; không có active_role thì cho qua.
func DenyActiveRole(code, message string, roles ...string) fiber.Handler {
	denied := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		denied[r] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("active_role").(string)
		if _, blocked := denied[role]; blocked {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": message, "code": code})
		}
		return c.Next()
	}
}
