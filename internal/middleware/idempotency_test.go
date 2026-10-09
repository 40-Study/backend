package middleware

// M2 (review 261009, "light fix"): header Idempotency-Key trên route gửi không hoàn tác được. Lặp lại cùng key của cùng
// một người trong TTL trả lại kết quả gốc mà KHÔNG chạy lại handler; thiếu header = hành vi cũ.
//
// Mutation đã thử (mỗi dòng làm ít nhất một test ĐỎ): bỏ SetNX (luôn chạy handler); bỏ Scope khỏi khoá Redis; luôn lưu
// kết quả kể cả khi Took=false; bỏ so khớp body; bỏ nhánh pending => 409.

import (
	"io"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type idemEnv struct {
	app   *fiber.App
	mr    *miniredis.Miniredis
	calls *int32
}

// idemApp: POST /send?result=ok|fail. ok => 201 {"n":<lần chạy>} (Took=true); fail => 400 (Took=false).
// Header X-Actor chọn Scope. Hộp thư phản hồi gồm số lần handler thật sự chạy.
func newIdemEnv(t *testing.T) *idemEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	calls := new(int32)
	idem := Idempotency(rdb, IdempotencyConfig{
		Header: "Idempotency-Key", KeyPrefix: "idem:test", TTL: 24 * time.Hour, PendingTTL: 10 * time.Minute,
		Scope: func(c *fiber.Ctx) string { return c.Get("X-Actor") },
		Took:  func(c *fiber.Ctx) bool { return c.Response().StatusCode() == fiber.StatusCreated },
	})
	app := fiber.New()
	app.Post("/send", idem, func(c *fiber.Ctx) error {
		n := atomic.AddInt32(calls, 1)
		if c.Query("result") == "fail" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "NOPE"})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": fiber.Map{"run": n}})
	})
	return &idemEnv{app: app, mr: mr, calls: calls}
}

type idemResp struct {
	status int
	body   string
	replay string
}

func (e *idemEnv) send(t *testing.T, actor, key, query, body string) idemResp {
	t.Helper()
	req := httptest.NewRequest("POST", "/send"+query, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor", actor)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return idemResp{status: resp.StatusCode, body: string(raw), replay: resp.Header.Get("Idempotent-Replay")}
}

func TestIdempotency_SameKeySameActor_ReplaysOriginalWithoutRerunning(t *testing.T) {
	e := newIdemEnv(t)
	first := e.send(t, "admin-a", "key-1", "", `{"x":1}`)
	if first.status != 201 || first.replay != "" {
		t.Fatalf("lần đầu = %+v, muốn 201 không có Idempotent-Replay", first)
	}
	second := e.send(t, "admin-a", "key-1", "", `{"x":1}`)
	if second.status != 201 || second.body != first.body || second.replay != "true" {
		t.Fatalf("lần lặp = %+v, muốn đúng kết quả gốc %+v kèm Idempotent-Replay: true", second, first)
	}
	if got := atomic.LoadInt32(e.calls); got != 1 {
		t.Fatalf("handler chạy %d lần, muốn 1 (lặp lại không được gửi lại)", got)
	}
}

func TestIdempotency_DifferentKeyOrActor_RunsAgain(t *testing.T) {
	e := newIdemEnv(t)
	e.send(t, "admin-a", "key-1", "", `{}`)
	e.send(t, "admin-a", "key-2", "", `{}`) // key khác
	e.send(t, "admin-b", "key-1", "", `{}`) // cùng key nhưng admin khác
	if got := atomic.LoadInt32(e.calls); got != 3 {
		t.Fatalf("handler chạy %d lần, muốn 3 (key khác hoặc admin khác là yêu cầu khác)", got)
	}
}

func TestIdempotency_NoHeader_BehavesAsBefore(t *testing.T) {
	e := newIdemEnv(t)
	for i := 0; i < 3; i++ {
		if r := e.send(t, "admin-a", "", "", `{}`); r.status != 201 || r.replay != "" {
			t.Fatalf("không header lần %d = %+v", i+1, r)
		}
	}
	if got := atomic.LoadInt32(e.calls); got != 3 {
		t.Fatalf("handler chạy %d lần, muốn 3", got)
	}
	if keys := e.mr.Keys(); len(keys) != 0 {
		t.Fatalf("không header thì không được ghi gì vào Redis: %v", keys)
	}
}

// Lần đầu thất bại (Took=false) không giữ khoá: sửa lỗi rồi gửi lại cùng key được chạy thật.
func TestIdempotency_FailedFirstAttempt_AllowsRetryWithSameKey(t *testing.T) {
	e := newIdemEnv(t)
	if r := e.send(t, "admin-a", "key-1", "?result=fail", `{}`); r.status != 400 {
		t.Fatalf("lần đầu = %+v, muốn 400", r)
	}
	if r := e.send(t, "admin-a", "key-1", "", `{}`); r.status != 201 || r.replay != "" {
		t.Fatalf("thử lại cùng key sau thất bại = %+v, muốn chạy thật (201)", r)
	}
	if got := atomic.LoadInt32(e.calls); got != 2 {
		t.Fatalf("handler chạy %d lần, muốn 2", got)
	}
}

func TestIdempotency_SameKeyDifferentBody_422(t *testing.T) {
	e := newIdemEnv(t)
	e.send(t, "admin-a", "key-1", "", `{"title":"A"}`)
	r := e.send(t, "admin-a", "key-1", "", `{"title":"B"}`)
	if r.status != 422 || !strings.Contains(r.body, "IDEMPOTENCY_KEY_REUSED") {
		t.Fatalf("cùng key khác nội dung = %+v, muốn 422 IDEMPOTENCY_KEY_REUSED", r)
	}
	if got := atomic.LoadInt32(e.calls); got != 1 {
		t.Fatalf("handler chạy %d lần, muốn 1", got)
	}
}

// Yêu cầu đầu còn đang chạy (khoá pending) thì yêu cầu trùng nhận 409, không chạy song song.
func TestIdempotency_RequestInFlight_409(t *testing.T) {
	e := newIdemEnv(t)
	started, release := make(chan struct{}), make(chan struct{})
	slow := fiber.New()
	rdb := redis.NewClient(&redis.Options{Addr: e.mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	slow.Post("/send", Idempotency(rdb, IdempotencyConfig{
		Header: "Idempotency-Key", KeyPrefix: "idem:test", TTL: time.Hour, PendingTTL: time.Minute,
		Scope: func(c *fiber.Ctx) string { return "admin-a" }, Took: func(*fiber.Ctx) bool { return true },
	}), func(c *fiber.Ctx) error {
		close(started)
		<-release
		return c.SendStatus(fiber.StatusCreated)
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest("POST", "/send", strings.NewReader(`{}`))
		req.Header.Set("Idempotency-Key", "key-1")
		if _, err := slow.Test(req, -1); err != nil {
			t.Error(err)
		}
	}()
	<-started
	req := httptest.NewRequest("POST", "/send", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", "key-1")
	resp, err := slow.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	close(release)
	<-done
	if resp.StatusCode != 409 || !strings.Contains(string(raw), "IDEMPOTENCY_IN_PROGRESS") {
		t.Fatalf("trùng khi đang chạy = %d %s, muốn 409 IDEMPOTENCY_IN_PROGRESS", resp.StatusCode, raw)
	}
}

func TestIdempotency_InvalidKey_400(t *testing.T) {
	e := newIdemEnv(t)
	r := e.send(t, "admin-a", strings.Repeat("k", 129), "", `{}`)
	if r.status != 400 || !strings.Contains(r.body, "IDEMPOTENCY_KEY_INVALID") {
		t.Fatalf("key 129 ký tự = %+v, muốn 400 IDEMPOTENCY_KEY_INVALID", r)
	}
	if atomic.LoadInt32(e.calls) != 0 {
		t.Fatal("key không hợp lệ mà handler vẫn chạy")
	}
}

// Redis lỗi: fail-closed 503 (giống RateLimiter), không âm thầm bỏ qua rồi gửi trùng.
func TestIdempotency_RedisDown_FailsClosed503(t *testing.T) {
	e := newIdemEnv(t)
	e.mr.Close()
	r := e.send(t, "admin-a", "key-1", "", `{}`)
	if r.status != 503 {
		t.Fatalf("Redis down = %+v, muốn 503", r)
	}
	if atomic.LoadInt32(e.calls) != 0 {
		t.Fatal("Redis down mà handler vẫn chạy (có thể gửi trùng)")
	}
}
