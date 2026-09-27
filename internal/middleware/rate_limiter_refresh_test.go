package middleware

// Test cho S-P1-2 (QA 260927): truoc ban va, /login VA /refresh-token dung CHUNG bucket
// "rate:auth" (Max 5/phut/IP, xem AuthRateLimiter) — verify-260927-student-admin.md xac nhan
// song "bi 429 o lan login dau tien do agent khac dung chung bucket". /refresh-token duoc web
// goi TU DONG (interceptor 401, nhieu tab/thiet bi cung IP) nen de tu dung tran roi khoa luon ca
// login that cua nguoi khac sau NAT/wifi chung.
//
// RefreshTokenRateLimiter (rate_limiter.go) tach bucket rieng "rate:refresh", Max 30/phut/IP.
// Test nay chung minh 2 dieu: (1) /refresh-token khong con bi khoa boi 5 request dau (nguong cua
// AuthRateLimiter) — phai toi 31 moi 429; (2) hai bucket DOC LAP — dung het AuthRateLimiter
// khong lam RefreshTokenRateLimiter bi anh huong va nguoc lai. Mutation: doi KeyPrefix cua
// RefreshTokenRateLimiter ve lai "rate:auth" (hoac bo han ham nay, dung chung authRateLimiter
// nhu truoc) se lam test nay do.

import (
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func newRateLimitTestApp(t *testing.T) (*fiber.App, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	app := fiber.New()
	noop := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	app.Post("/login", AuthRateLimiter(rdb), noop)
	app.Post("/refresh-token", RefreshTokenRateLimiter(rdb), noop)
	return app, rdb
}

func callN(t *testing.T, app *fiber.App, path string, n int) int {
	t.Helper()
	var last int
	for i := 0; i < n; i++ {
		req := httptest.NewRequest("POST", path, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test loi: %v", err)
		}
		last = resp.StatusCode
	}
	return last
}

// TestRefreshTokenRateLimiter_RongHonLogin_31LanMoi429 (S-P1-2): 30 lan goi lien tiep
// /refresh-token tu CUNG IP phai deu KHONG 429 (con trong nguong 30/phut); lan thu 31 moi 429.
func TestRefreshTokenRateLimiter_RongHonLogin_31LanMoi429(t *testing.T) {
	app, _ := newRateLimitTestApp(t)

	for i := 1; i <= 30; i++ {
		req := httptest.NewRequest("POST", "/refresh-token", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test loi: %v", err)
		}
		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Fatalf("lan goi thu %d (trong nguong 30) da bi 429 — nguong RefreshTokenRateLimiter qua chat hoac dung nham bucket auth", i)
		}
	}

	status := callN(t, app, "/refresh-token", 1)
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("lan goi thu 31, status = %d, muon 429", status)
	}
}

// TestLoginRateLimiter_VanGiu5LanMoi429 (S-P1-2): /login KHONG doi hanh vi — van 429 tu lan thu 6.
func TestLoginRateLimiter_VanGiu5LanMoi429(t *testing.T) {
	app, _ := newRateLimitTestApp(t)

	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test loi: %v", err)
		}
		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Fatalf("lan goi thu %d (trong nguong 5) cua /login da bi 429", i)
		}
	}
	status := callN(t, app, "/login", 1)
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("lan goi thu 6 cua /login, status = %d, muon 429", status)
	}
}

// TestAuthVaRefreshRateLimiter_HaiBucketDocLap (S-P1-2, cot loi cua bug goc): dung het bucket
// /login (5 lan) KHONG duoc lam /refresh-token bi khoa theo — day chinh la trieu chung goc bi
// verify-260927-student-admin.md ghi nhan (agent khac login lam agent nay bi 429 refresh-token).
func TestAuthVaRefreshRateLimiter_HaiBucketDocLap(t *testing.T) {
	app, _ := newRateLimitTestApp(t)

	// Dung can nguong /login (5 lan, lan thu 6 se 429 nhung khong quan tam ket qua o day).
	callN(t, app, "/login", 6)

	// /refresh-token tu CUNG IP phai KHONG bi anh huong — con nguyen nguong rieng 30 lan.
	req := httptest.NewRequest("POST", "/refresh-token", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode == fiber.StatusTooManyRequests {
		t.Fatal("/refresh-token bi 429 NGAY LAN GOI DAU TIEN chi vi /login (cung IP) da het nguong — hai bucket dang dung chung key, dung dung bug S-P1-2")
	}
}
