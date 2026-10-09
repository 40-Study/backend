package middleware

// M4 (review 261009): hạn mức "5 lần gửi / giờ" chỉ được tiêu bởi lần gửi THỰC SỰ có tác dụng. INCR vẫn chạy trước
// handler (nguyên tử khi song song) và RateLimitConfig.Refund hoàn lượt khi handler báo không có tác dụng.
//
// Mutation đã thử: bỏ khối hoàn lượt trong RateLimiter; hoàn lượt cả khi 429; bỏ DEL khi DECR về âm.

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

const refundTestMax = 2

// newRefundApp: POST /act?ok=1 "có tác dụng", mọi giá trị khác là thất bại (400). Refund khi không có ok=1.
func newRefundApp(t *testing.T) (*fiber.App, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	limiter := RateLimiter(rdb, RateLimitConfig{
		Max: refundTestMax, Window: time.Hour, KeyPrefix: "rate:refund-test",
		KeyGenerator: func(*fiber.Ctx) string { return "admin-1" },
		Refund:       func(c *fiber.Ctx) bool { return c.Query("ok") != "1" },
	})
	app := fiber.New()
	app.Post("/act", limiter, func(c *fiber.Ctx) error {
		if c.Query("ok") == "1" {
			return c.SendStatus(fiber.StatusCreated)
		}
		return c.SendStatus(fiber.StatusBadRequest)
	})
	return app, mr
}

func actStatus(t *testing.T, app *fiber.App, ok bool) int {
	t.Helper()
	target := "/act"
	if ok {
		target += "?ok=1"
	}
	resp, err := app.Test(httptest.NewRequest("POST", target, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

func TestRateLimiter_Refund_FailedRequestsDoNotConsumeQuota(t *testing.T) {
	app, _ := newRefundApp(t)
	for i := 0; i < refundTestMax*3; i++ {
		if got := actStatus(t, app, false); got != fiber.StatusBadRequest {
			t.Fatalf("lần thất bại %d = %d, muốn 400 (không bao giờ 429 khi chưa có lần nào thành công)", i+1, got)
		}
	}
	for i := 1; i <= refundTestMax; i++ {
		if got := actStatus(t, app, true); got != fiber.StatusCreated {
			t.Fatalf("lần thành công %d = %d, muốn 201", i, got)
		}
	}
	if got := actStatus(t, app, true); got != fiber.StatusTooManyRequests {
		t.Fatalf("lần thành công thứ %d = %d, muốn 429", refundTestMax+1, got)
	}
}

// Thất bại xen kẽ giữa các lần thành công không làm lệch bộ đếm; request bị 429 không được hoàn (không tự mở khoá).
func TestRateLimiter_Refund_InterleavedAndBlockedRequests(t *testing.T) {
	app, mr := newRefundApp(t)
	actStatus(t, app, true)
	actStatus(t, app, false)
	actStatus(t, app, true)
	actStatus(t, app, false)
	if got := actStatus(t, app, true); got != fiber.StatusTooManyRequests {
		t.Fatalf("sau 2 lần thành công, lần thứ 3 = %d, muốn 429", got)
	}
	// Lần bị chặn gửi dạng thất bại cũng không được hoàn quota đã bị chặn: bộ đếm không được tụt xuống dưới Max.
	for i := 0; i < 5; i++ {
		if got := actStatus(t, app, false); got != fiber.StatusTooManyRequests {
			t.Fatalf("khi đã hết hạn mức, request thất bại thứ %d = %d, muốn 429", i+1, got)
		}
	}
	if v, err := mr.Get("rate:refund-test:admin-1"); err != nil || v == "" {
		t.Fatalf("key đếm phải còn tồn tại: %q %v", v, err)
	}
}

// Key hết hạn giữa INCR và DECR không được để lại bộ đếm âm vĩnh viễn không TTL.
func TestRateLimiter_Refund_ExpiredKeyDoesNotLeaveNegativeCounter(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	limiter := RateLimiter(rdb, RateLimitConfig{
		Max: refundTestMax, Window: time.Hour, KeyPrefix: "rate:refund-test",
		KeyGenerator: func(*fiber.Ctx) string { return "admin-1" },
		Refund:       func(*fiber.Ctx) bool { return true },
	})
	app := fiber.New()
	app.Post("/act", limiter, func(c *fiber.Ctx) error {
		mr.FastForward(2 * time.Hour) // cửa sổ hết hạn trong lúc handler chạy: key bị xoá
		return c.SendStatus(fiber.StatusBadRequest)
	})
	if resp, err := app.Test(httptest.NewRequest("POST", "/act", nil), -1); err != nil || resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("request: %v %v", resp, err)
	}
	if mr.Exists("rate:refund-test:admin-1") {
		v, _ := mr.Get("rate:refund-test:admin-1")
		t.Fatalf("key đếm còn lại với giá trị %q sau khi hết hạn + hoàn lượt (sẽ không bao giờ hết hạn)", v)
	}
}
