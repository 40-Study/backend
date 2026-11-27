package handler

// QA8 review L1: chỉ OTP bị từ chối THẬT mới tính vào bộ đếm khoá; lỗi DB/Redis của Register trả 500 chung, không lộ lỗi gốc
// và không khoá tài khoản. Bỏ nhánh `!errors.Is(err, service.ErrRegisterOTPRejected)` trong AuthHandler.Register thì ĐỎ
// (lỗi DB thành 400 kèm chuỗi pq và lần thứ 11 bị 429).

import (
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

func newRegisterLockoutApp(t *testing.T, err error) *fiber.App {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	lockout := middleware.AccountFailureLockout(rdb, middleware.AccountLockoutConfig{KeyPrefix: "t:qa8reg", MaxFailures: 10})
	app := fiber.New()
	app.Post("/register", lockout, NewAuthHandler(&registerStub{err: err}).Register)
	return app
}

func postRegister(t *testing.T, app *fiber.App) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/register", strings.NewReader(`{"email":"victim@40study.test","otp":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestQA8Review_Register_DBErrorNotCountedNotLeaked(t *testing.T) {
	app := newRegisterLockoutApp(t, errors.New("failed to create user: pq: connection reset SECRETDETAIL"))
	for i := 1; i <= 11; i++ {
		code, body := postRegister(t, app)
		if code != fiber.StatusInternalServerError || strings.Contains(body, "SECRETDETAIL") {
			t.Fatalf("lần %d: status=%d body=%s, muốn 500 chung không lộ lỗi gốc (không bị khoá 429)", i, code, body)
		}
	}
}

func TestQA8Review_Register_RejectedOTPStillCounted(t *testing.T) {
	app := newRegisterLockoutApp(t, fmt.Errorf("%w: invalid OTP", service.ErrRegisterOTPRejected))
	for i := 1; i <= 10; i++ {
		if code, body := postRegister(t, app); code != fiber.StatusBadRequest {
			t.Fatalf("lần %d: status=%d body=%s, muốn 400", i, code, body)
		}
	}
	if code, _ := postRegister(t, app); code != fiber.StatusTooManyRequests {
		t.Fatalf("lần 11 sau 10 OTP sai: status=%d, muốn 429", code)
	}
}
