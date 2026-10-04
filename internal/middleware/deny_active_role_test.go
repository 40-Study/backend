package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Phụ huynh không dùng ví xu: chặn theo vai ĐANG CHỌN. Bỏ DenyActiveRole thì ca PARENT ĐỎ; chặn theo mọi vai người dùng
// giữ thì ca STUDENT (tài khoản vừa học viên vừa phụ huynh) ĐỎ.
func TestDenyActiveRole(t *testing.T) {
	for _, tc := range []struct {
		activeRole string
		want       int
	}{
		{"PARENT", fiber.StatusForbidden},
		{"STUDENT", fiber.StatusOK},
		{"TEACHER", fiber.StatusOK},
		{"", fiber.StatusOK}, // không có active_role: để AuthMiddleware/permission khác quyết định
	} {
		app := fiber.New()
		app.Use(func(c *fiber.Ctx) error {
			if tc.activeRole != "" {
				c.Locals("active_role", tc.activeRole)
			}
			return c.Next()
		})
		app.Use(DenyActiveRole("COIN_WALLET_NOT_FOR_PARENT", "msg", "PARENT"))
		app.Get("/wallet", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

		res, err := app.Test(httptest.NewRequest("GET", "/wallet", nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != tc.want {
			t.Errorf("active_role=%q: status=%d, muốn %d", tc.activeRole, res.StatusCode, tc.want)
		}
		if tc.want == fiber.StatusForbidden {
			buf := new(strings.Builder)
			b := make([]byte, 256)
			n, _ := res.Body.Read(b)
			buf.Write(b[:n])
			if !strings.Contains(buf.String(), `"code":"COIN_WALLET_NOT_FOR_PARENT"`) {
				t.Errorf("thân 403 thiếu code: %s", buf.String())
			}
		}
	}
}
