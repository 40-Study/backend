package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func newSameOriginTestApp(allowedOrigins string) *fiber.App {
	app := fiber.New()
	app.Post("/progress", SameOriginRequired(allowedOrigins), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

// withAccessTokenCookie (N3, review vong 2 260915): sau N3, SameOriginRequired CHI kiem tra khi
// request mang cookie "accessToken" — mo phong dung request cookie-authenticated ma CSRF nham
// toi. Cac test kiem tra logic Origin/Referer ben duoi deu can cookie nay de khong bi bo qua
// som boi nhanh N3.
func withAccessTokenCookie(req *http.Request) *http.Request {
	req.AddCookie(&http.Cookie{Name: "accessToken", Value: "dummy-token-cho-test"})
	return req
}

// TestSameOriginRequired_AllowedOriginPasses (MED-3, review 260915): Origin nam trong
// ALLOWED_ORIGINS thi request phai toi duoc handler ke tiep.
func TestSameOriginRequired_AllowedOriginPasses(t *testing.T) {
	app := newSameOriginTestApp("https://40study.example,http://localhost:3000")

	req := httptest.NewRequest("POST", "/progress", nil)
	req.Header.Set("Origin", "https://40study.example")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200 (origin hop le phai duoc cho qua)", res.StatusCode)
	}
}

// TestSameOriginRequired_DisallowedOriginRejected: mot trang la gui Origin cua no —
// day chinh la kich ban CSRF ma finding MED-3 mo ta (sendBeacon tu trang tan cong toi
// route cookie-auth).
func TestSameOriginRequired_DisallowedOriginRejected(t *testing.T) {
	app := newSameOriginTestApp("https://40study.example")

	req := withAccessTokenCookie(httptest.NewRequest("POST", "/progress", nil))
	req.Header.Set("Origin", "https://evil.example")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 cho origin la", res.StatusCode)
	}
}

// TestSameOriginRequired_MissingOriginAndReferer_Rejected: khong co Origin lan Referer, NHUNG
// CO cookie accessToken (request cookie-authenticated that su, N3) — tu choi thay vi coi la hop
// le.
func TestSameOriginRequired_MissingOriginAndReferer_Rejected(t *testing.T) {
	app := newSameOriginTestApp("https://40study.example")

	req := withAccessTokenCookie(httptest.NewRequest("POST", "/progress", nil))
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 khi thieu ca Origin lan Referer", res.StatusCode)
	}
}

// TestSameOriginRequired_RefererFallback_Allowed: mot so trinh duyet/tinh huong sendBeacon
// cung origin khong dat Origin nhung van dat Referer — phai duoc rut scheme+host tu Referer
// va cho qua khi khop danh sach.
func TestSameOriginRequired_RefererFallback_Allowed(t *testing.T) {
	app := newSameOriginTestApp("https://40study.example")

	req := withAccessTokenCookie(httptest.NewRequest("POST", "/progress", nil))
	req.Header.Set("Referer", "https://40study.example/courses/abc/learn")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200 (Referer cung scheme+host voi origin hop le)", res.StatusCode)
	}
}

// TestSameOriginRequired_RefererFallback_Rejected: Referer tro toi mot host khac van bi tu choi
// — chung minh fallback co doi chieu that, khong phai luon cho qua khi co Referer.
func TestSameOriginRequired_RefererFallback_Rejected(t *testing.T) {
	app := newSameOriginTestApp("https://40study.example")

	req := withAccessTokenCookie(httptest.NewRequest("POST", "/progress", nil))
	req.Header.Set("Referer", "https://evil.example/attack")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 (Referer tro toi host khac)", res.StatusCode)
	}
}

// TestSameOriginRequired_WildcardSkipsCheck: ALLOWED_ORIGINS=* (vd moi truong dev) tat han
// che nay, giong het cach middleware cors.New xu ly "*".
func TestSameOriginRequired_WildcardSkipsCheck(t *testing.T) {
	app := newSameOriginTestApp("*")

	req := httptest.NewRequest("POST", "/progress", nil)
	req.Header.Set("Origin", "https://khong-lien-quan.example")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200 khi ALLOWED_ORIGINS=*", res.StatusCode)
	}
}

// TestSameOriginRequired_BearerKhongCookie_BoQuaKiemOrigin (N3, review vong 2 260915): client
// dung Authorization: Bearer (mobile Flutter, curl, server-to-server) — KHONG dat cookie
// accessToken, KHONG dat Origin/Referer (khong phai trinh duyet). Truoc N3, request nay bi 403
// OAN du token hoan toan hop le, vi CSRF khong the xay ra voi Bearer (JS cua mot trang la khong
// the doc duoc token de tu gan vao Authorization header).
func TestSameOriginRequired_BearerKhongCookie_BoQuaKiemOrigin(t *testing.T) {
	app := newSameOriginTestApp("https://40study.example")

	req := httptest.NewRequest("POST", "/progress", nil)
	req.Header.Set("Authorization", "Bearer some.valid.jwt")
	// KHONG dat Origin, KHONG dat Referer, KHONG dat cookie.
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200 (Bearer khong co cookie -> khong phai CSRF, bo qua kiem origin)", res.StatusCode)
	}
}

// TestSameOriginRequired_KhongCookieKhongBearer_BoQuaKiemOrigin: request hoan toan khong auth
// gi (khong cookie, khong Bearer) cung phai duoc middleware nay cho qua — AuthMiddleware phia
// sau se tu 401. SameOriginRequired chi lo phan CSRF, khong lo phan auth.
func TestSameOriginRequired_KhongCookieKhongBearer_BoQuaKiemOrigin(t *testing.T) {
	app := newSameOriginTestApp("https://40study.example")

	req := httptest.NewRequest("POST", "/progress", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200 (khong cookie thi khong phai duong CSRF, de AuthMiddleware phia sau tu quyet dinh)", res.StatusCode)
	}
}

// TestSameOriginRequired_CoCaCookieLanBearer_VanKiemOrigin: request mang CA HAI cookie va
// Bearer (khong thuc te lam nhung phai dung an toan) — AuthMiddleware uu tien cookie, nen day
// van la duong CSRF phai kiem, khong duoc bo qua chi vi co Bearer di kem.
func TestSameOriginRequired_CoCaCookieLanBearer_VanKiemOrigin(t *testing.T) {
	app := newSameOriginTestApp("https://40study.example")

	req := withAccessTokenCookie(httptest.NewRequest("POST", "/progress", nil))
	req.Header.Set("Authorization", "Bearer some.valid.jwt")
	req.Header.Set("Origin", "https://evil.example")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 (co cookie -> van la duong CSRF du co Bearer di kem)", res.StatusCode)
	}
}
