package middleware

// Test cho S-P1-2 (QA 260927, bổ sung sau khi PR #69 mở) — mở rộng phạm vi so với bản đầu (chỉ
// tách /refresh-token). Coordinator chỉ ra: /select-role và /select-org VẪN dùng chung bucket
// "rate:auth" (AuthRateLimiter, 5/phút/IP) với /login. Một lần đăng nhập TRỌN VẸN đã là
// login + select-role (+ select-org nếu đổi tổ chức) — tức tiêu 2-3 lượt trong CHÍNH bucket
// 5-lượt/phút đó, nên một mạng dùng chung IP (trường học, văn phòng) chỉ đăng nhập trọn được
// khoảng 2 lần/phút thay vì 5. select-role/select-org không phải bề mặt dò mật khẩu (đã qua
// bước xác thực mật khẩu ở /login, hoặc đã cầm access token thật) nên được chuyển sang
// WideAuthRateLimiter — CÙNG bucket "rate:post-auth" (30/phút/IP) mà /refresh-token đã dùng.
//
// Test nay chung minh: (1) /select-role, /select-org, /refresh-token CHIA SE 1 bucket rong,
// doc lap voi /login; (2) mo phong dung kich ban bug goc — goi xen ke login+select-role nhieu
// lan lien tiep (nhu nguoi dung that dang nhap lai nhieu lan) — select-role KHONG duoc 429 du
// /login da cham nguong rieng cua no. Mutation: gan lai authRateLimiter (5/phut) cho select-role/
// select-org trong auth_router.go se lam nhom test nay do.

import (
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func newPostAuthRateLimitTestApp(t *testing.T) *fiber.App {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	// Cung 1 instance postAuthRateLimiter cho ca 3 route — dung dung cach auth_router.go noi
	// day: mot bien postAuthRateLimiter duoc gan cho ca select-role, select-org, refresh-token.
	postAuthRateLimiter := WideAuthRateLimiter(rdb, "rate:post-auth", 30)

	app := fiber.New()
	noop := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	app.Post("/login", AuthRateLimiter(rdb), noop)
	app.Post("/select-role", postAuthRateLimiter, noop)
	app.Post("/select-org", postAuthRateLimiter, noop)
	app.Post("/refresh-token", postAuthRateLimiter, noop)
	return app
}

func postN(t *testing.T, app *fiber.App, path string, n int) int {
	t.Helper()
	var last int
	for i := 0; i < n; i++ {
		resp, err := app.Test(httptest.NewRequest("POST", path, nil))
		if err != nil {
			t.Fatalf("app.Test loi: %v", err)
		}
		last = resp.StatusCode
	}
	return last
}

// TestPostAuthRateLimiter_30LanMoi429: ca 3 route select-role/select-org/refresh-token deu dung
// chung bucket "rate:post-auth" — 30 lan dau (tren CA 3 route CONG LAI, vi cung 1 key theo IP)
// khong duoc 429; lan thu 31 moi 429.
func TestPostAuthRateLimiter_30LanMoi429(t *testing.T) {
	app := newPostAuthRateLimitTestApp(t)

	paths := []string{"/select-role", "/select-org", "/refresh-token"}
	for i := 1; i <= 30; i++ {
		path := paths[i%len(paths)]
		resp, err := app.Test(httptest.NewRequest("POST", path, nil))
		if err != nil {
			t.Fatalf("app.Test loi: %v", err)
		}
		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Fatalf("lan goi thu %d (%s, trong nguong 30) da bi 429 — nguong qua chat hoac 3 route khong con chung bucket", i, path)
		}
	}
	status := postN(t, app, "/select-role", 1)
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("lan goi thu 31, status = %d, muon 429", status)
	}
}

// TestLoginRateLimiter_VanGiu5LanMoi429: /login khong doi hanh vi — van 429 tu lan thu 6.
func TestLoginRateLimiter_VanGiu5LanMoi429(t *testing.T) {
	app := newPostAuthRateLimitTestApp(t)

	for i := 1; i <= 5; i++ {
		resp, err := app.Test(httptest.NewRequest("POST", "/login", nil))
		if err != nil {
			t.Fatalf("app.Test loi: %v", err)
		}
		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Fatalf("lan goi thu %d (trong nguong 5) cua /login da bi 429", i)
		}
	}
	status := postN(t, app, "/login", 1)
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("lan goi thu 6 cua /login, status = %d, muon 429", status)
	}
}

// TestAuthVaPostAuthRateLimiter_HaiBucketDocLap: dung het bucket /login (6 lan) KHONG duoc lam
// /select-role hay /refresh-token bi khoa theo — day la trieu chung goc S-P1-2.
func TestAuthVaPostAuthRateLimiter_HaiBucketDocLap(t *testing.T) {
	app := newPostAuthRateLimitTestApp(t)

	postN(t, app, "/login", 6)

	for _, path := range []string{"/select-role", "/select-org", "/refresh-token"} {
		resp, err := app.Test(httptest.NewRequest("POST", path, nil))
		if err != nil {
			t.Fatalf("app.Test loi: %v", err)
		}
		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Fatalf("%s bi 429 chi vi /login (cung IP) da het nguong rieng cua no — hai bucket dang dung chung key", path)
		}
	}
}

// TestDangNhapLapLai_LoginVaSelectRoleXenKe_SelectRoleKhongTuChanChinhMinh: mo phong DUNG kich
// ban bug goc coordinator neu — mot nguoi dung (hoac nhieu nguoi cung IP mang truong/van phong)
// dang nhap TRON VEN (login roi select-role) 5 lan lien tiep trong 1 phut. Truoc ban va bo sung
// nay, ca hai cung tieu chung 1 bucket 5-luot/phut nen lan dang nhap thu 3 da bi chan (login lan
// 3 la request thu 5 trong bucket, select-role lan 3 la request thu 6 -> 429). Sau ban va,
// select-role dung bucket rieng rong hon nen khong bao gio bi chan boi chinh /login.
func TestDangNhapLapLai_LoginVaSelectRoleXenKe_SelectRoleKhongTuChanChinhMinh(t *testing.T) {
	app := newPostAuthRateLimitTestApp(t)

	completedWithoutSelectRole429 := 0
	for round := 1; round <= 5; round++ {
		// Khong quan tam /login co bi 429 khong o vong nay (no co nguong rieng 5/phut cua no,
		// khong phai thu dang kiem trong test nay) — chi quan tam select-role.
		_, _ = app.Test(httptest.NewRequest("POST", "/login", nil))

		resp, err := app.Test(httptest.NewRequest("POST", "/select-role", nil))
		if err != nil {
			t.Fatalf("app.Test loi: %v", err)
		}
		if resp.StatusCode == fiber.StatusTooManyRequests {
			t.Fatalf("vong dang nhap thu %d: /select-role bi 429 — 1 nguoi dung (hoac IP dung chung) khong dang nhap tron ven duoc qua %d lan/phut", round, completedWithoutSelectRole429)
		}
		completedWithoutSelectRole429++
	}
	if completedWithoutSelectRole429 != 5 {
		t.Fatalf("chi hoan tat %d/5 vong dang nhap ma khong bi 429 o select-role, muon ca 5", completedWithoutSelectRole429)
	}
}
