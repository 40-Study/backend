package router

// QA6-PG-01 (C6): phụ huynh (vai đang chọn) bị 403 ERR_PARENT_CANNOT_PURCHASE ở ROUTER thật khi đăng ký khoá học,
// thêm giỏ hàng, tạo đơn. Gỡ parentCannotPurchase() khỏi một trong ba route thì ca PARENT của route đó ĐỎ (rơi về handler:
// 400/500, không còn 403); chặn theo mọi vai người dùng giữ thì ca STUDENT ĐỎ (403 sai).

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/utils"
)

func TestQA6_PhuHuynhKhongMuaKhoaHoc_QuaRouterThat(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "qa6-parent-purchase-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	app := fiber.New()
	app.Use(fiberrecover.New())
	api := app.Group("/api")
	// Handler thật với service nil: ca bị chặn không bao giờ chạm tới service; ca học viên rơi vào handler và panic -> 500 (recover).
	SetupEnrollmentRoutes(api, cfg, handler.NewEnrollmentHandler(nil, nil), rdb)
	SetupCartRoutes(api, cfg, handler.NewCartHandler(nil), rdb)
	SetupOrderRoutes(api, cfg, handler.NewOrderHandler(nil, nil, nil), nil, rdb, middleware.NewPermissionChecker(nil, nil, nil, nil), nil)

	token := func(role string) string {
		uid := uuid.New()
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(uid.String()), 1, 0).Err(); err != nil {
			t.Fatal(err)
		}
		tok, _, err := utils.GenerateTokens(cfg, uid, uuid.New(), role, nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	do := func(tok, method, path, body string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}

	routes := [][3]string{
		{"POST", "/api/courses/" + uuid.NewString() + "/enroll", `{}`},
		{"POST", "/api/cart/", `{"course_id":"` + uuid.NewString() + `"}`},
		{"POST", "/api/orders/", `{}`},
	}
	parent, student := token("PARENT"), token("STUDENT")
	for _, r := range routes {
		status, raw := do(parent, r[0], r[1], r[2])
		if status != fiber.StatusForbidden || !strings.Contains(raw, `"code":"ERR_PARENT_CANNOT_PURCHASE"`) {
			t.Errorf("PARENT %s %s: %d %s, muốn 403 kèm code ERR_PARENT_CANNOT_PURCHASE", r[0], r[1], status, raw)
		}
		if status, raw := do(student, r[0], r[1], r[2]); status == fiber.StatusForbidden {
			t.Errorf("STUDENT %s %s bị chặn 403: %s", r[0], r[1], raw)
		}
	}
}
