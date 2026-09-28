package middleware

// Review vòng 3 (plans/reports/review-260928-round3-pr69-pr25.md): BLOCKER có điều kiện — nếu
// cổng backend từng lộ trực tiếp không qua nginx, client tự gửi X-Forwarded-For, Next `??=` giữ
// nguyên (không ghi đè) nên client xoay XFF liên tục né hoàn toàn rate-limit theo IP dù ClientIP()
// đã đúng thuật toán. Test then chốt (TestAccountFailureLockout_XoayXFFMoiRequest_VanBiKhoaSauNLanSai)
// mô phỏng ĐÚNG kịch bản đó: ghép AuthRateLimiter (IP) + AccountFailureLockout (email) trên cùng
// route, mỗi request 1 X-Forwarded-For khác nhau (né hoàn toàn bucket IP) — vẫn phải bị khoá sau
// đúng ngưỡng, vì AccountFailureLockout không đọc IP/XFF.

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func newAccountLockoutTestApp(t *testing.T, cfg AccountLockoutConfig, withIPRateLimiter bool) (*fiber.App, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	app := fiber.New()
	// loginHandler mo phong DUNG hanh vi handler that (auth_handler.go#Login): request o day
	// coi nhu DA qua BodyParser+ValidateStruct (test nay tap trung vao co che dem/khoa/reset cua
	// chinh middleware, khong phai vao ranh gioi "request rac vs xac thuc sai that" -- ranh gioi
	// do da co bo test rieng dung handler THAT o internal/handler/account_lockout_real_handler_test.go)
	// nen luon dat co AuthCredentialRejectedLocalsKey khi sai mat khau, giong het diem handler
	// that dat co ngay sau khi goi service that va nhan loi.
	loginHandler := func(c *fiber.Ctx) error {
		var body struct {
			Password string `json:"password"`
		}
		_ = c.BodyParser(&body)
		if body.Password == "correct-password" {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Login successful"})
		}
		c.Locals(AuthCredentialRejectedLocalsKey, true)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Login failed"})
	}

	lockout := AccountFailureLockout(rdb, cfg)
	if withIPRateLimiter {
		// app.Test() cua Fiber luon gia lap TCP peer la "0.0.0.0" (helpers.go) -- coi peer nay la
		// dang tin de AuthRateLimiter thuc su doc X-Forwarded-For thay vi bo qua.
		trusted := NewTrustedProxySet([]string{"0.0.0.0"})
		app.Post("/login", AuthRateLimiter(rdb, trusted), lockout, loginHandler)
	} else {
		app.Post("/login", lockout, loginHandler)
	}
	return app, rdb
}

func postLogin(t *testing.T, app *fiber.App, email, password, xff string) int {
	t.Helper()
	body := `{"email":"` + email + `","password":"` + password + `"}`
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if xff != "" {
		req.Header.Set(fiber.HeaderXForwardedFor, xff)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	return resp.StatusCode
}

func TestAccountFailureLockout_KhoaSauNguongThatBai(t *testing.T) {
	app, _ := newAccountLockoutTestApp(t, AccountLockoutConfig{KeyPrefix: "t:lockout1", MaxFailures: 10}, false)

	for i := 1; i <= 10; i++ {
		status := postLogin(t, app, "victim@demo.com", "wrong-password", "")
		if status != fiber.StatusUnauthorized {
			t.Fatalf("lan %d: status = %d, muon 401 (chua toi nguong)", i, status)
		}
	}

	status := postLogin(t, app, "victim@demo.com", "wrong-password", "")
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("lan 11 (sau 10 lan sai): status = %d, muon 429 (da bi khoa)", status)
	}
}

func TestAccountFailureLockout_XoayXFFMoiRequest_VanBiKhoaSauNLanSai(t *testing.T) {
	app, _ := newAccountLockoutTestApp(t, AccountLockoutConfig{KeyPrefix: "t:lockout2", MaxFailures: 10}, true)

	// Moi request 1 X-Forwarded-For KHAC NHAU -- neu chi co AuthRateLimiter (IP) thi khong bao
	// gio bi 429 vi moi request roi vao 1 bucket IP rieng. AccountFailureLockout (email) khong
	// doc IP/XFF nen van phai khoa dung sau 10 lan.
	for i := 1; i <= 10; i++ {
		xff := "203.0.113." + strconv.Itoa(i)
		status := postLogin(t, app, "victim@demo.com", "wrong-password", xff)
		if status != fiber.StatusUnauthorized {
			t.Fatalf("lan %d (xff=%s): status = %d, muon 401 -- IP-limiter khong duoc chan (bucket IP khac nhau moi lan)", i, xff, status)
		}
	}

	// Lan 11, lai 1 XFF hoan toan moi (chua bao gio dung) -- neu chi dua vao IP se KHONG bi chan,
	// nhung account lockout phai chan vi da du 10 lan sai tren CUNG 1 email.
	status := postLogin(t, app, "victim@demo.com", "wrong-password", "203.0.113.999")
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("lan 11 (XFF moi hoan toan, chua tung dung): status = %d, muon 429 -- account lockout phai doc lap voi IP/XFF", status)
	}
}

func TestAccountFailureLockout_DangNhapDung_ResetBoDem(t *testing.T) {
	app, _ := newAccountLockoutTestApp(t, AccountLockoutConfig{KeyPrefix: "t:lockout3", MaxFailures: 10}, false)

	for i := 1; i <= 9; i++ {
		postLogin(t, app, "victim@demo.com", "wrong-password", "")
	}
	// Dang nhap dung o lan thu 10 -- phai reset bo dem thay vi cong don toi nguong.
	status := postLogin(t, app, "victim@demo.com", "correct-password", "")
	if status != fiber.StatusOK {
		t.Fatalf("dang nhap dung phai tra 200, nhan %d", status)
	}

	// 9 lan sai tiep theo (tong cong da 18 lan sai neu KHONG reset) -- neu bo dem da reset dung,
	// day chi la lan 1-9 moi, KHONG duoc bi khoa.
	for i := 1; i <= 9; i++ {
		s := postLogin(t, app, "victim@demo.com", "wrong-password", "")
		if s != fiber.StatusUnauthorized {
			t.Fatalf("sau khi dang nhap dung, lan sai thu %d (moi) status = %d, muon 401 (bo dem phai da reset, chua toi nguong)", i, s)
		}
	}
}

func TestAccountFailureLockout_HaiEmailKhacNhau_DocLap(t *testing.T) {
	app, _ := newAccountLockoutTestApp(t, AccountLockoutConfig{KeyPrefix: "t:lockout4", MaxFailures: 10}, false)

	for i := 1; i <= 11; i++ {
		postLogin(t, app, "victim-a@demo.com", "wrong-password", "")
	}
	// victim-a da bi khoa (lan 11 tra 429) -- victim-b hoan toan chua bi dung toi, phai con binh
	// thuong (401, khong phai 429).
	status := postLogin(t, app, "victim-b@demo.com", "wrong-password", "")
	if status != fiber.StatusUnauthorized {
		t.Fatalf("victim-b (email khac, chua tung sai lan nao) status = %d, muon 401 -- khong duoc an huong boi khoa cua victim-a", status)
	}
}

func TestAccountFailureLockout_EmailKhongTonTai_CungHanhViCungThongBao(t *testing.T) {
	// Khong tiet lo email co ton tai hay khong: middleware nay khong tu kiem tra ton tai, chi
	// dem status tra ve (handler that cung tra 401 cho ca sai mat khau LAN sai email, nen 2
	// truong hop nay khong the phan biet duoc o day) -- xac nhan hanh vi khoa GIONG HET nhau.
	app, _ := newAccountLockoutTestApp(t, AccountLockoutConfig{KeyPrefix: "t:lockout5", MaxFailures: 10}, false)

	for i := 1; i <= 10; i++ {
		postLogin(t, app, "khong-ton-tai@demo.com", "bat-ky-mat-khau", "")
	}
	status := postLogin(t, app, "khong-ton-tai@demo.com", "bat-ky-mat-khau", "")
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("email khong ton tai van phai bi khoa dung sau 10 lan, status = %d, muon 429", status)
	}
}

func TestAccountFailureLockout_ChuanHoaEmail_HoaThuongVaKhoangTrangCungBucket(t *testing.T) {
	app, _ := newAccountLockoutTestApp(t, AccountLockoutConfig{KeyPrefix: "t:lockout6", MaxFailures: 10}, false)

	variants := []string{
		"Victim@Demo.com", " victim@demo.com", "VICTIM@DEMO.COM ", "victim@demo.com",
	}
	for i := 0; i < 10; i++ {
		postLogin(t, app, variants[i%len(variants)], "wrong-password", "")
	}
	status := postLogin(t, app, "victim@demo.com", "wrong-password", "")
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("cac bien the hoa/thuong + khoang trang cua cung 1 email phai dung chung 1 bucket, status = %d, muon 429", status)
	}
}
