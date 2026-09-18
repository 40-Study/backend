package router

// Test cho hai lop bao ve tren POST /api/progress duoc them ngay 2026-09-15 (review 260915):
//   MED-3: SameOriginRequired chan CSRF cho simple request (sendBeacon text/plain).
//   MED-5: middleware auth van gan tren route nay, khong chi dua vao extractUserID cua handler.
//
// Ca hai deu can mount qua SetupEnrollmentRoutes that (khong phai mount tay tung
// handler nhu progress_quiz_routes_test.go) de kiem duoc CHINH THU TU middleware
// dang chay trong router that.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
)

const testAllowedOrigin = "https://40study.example"

func mountEnrollmentRoutesForOriginTest() *fiber.App {
	app := fiber.New()
	api := app.Group("/api")
	cfg := &config.Config{AllowedOrigins: testAllowedOrigin}
	SetupEnrollmentRoutes(api, cfg, handler.NewEnrollmentHandler(&fakeEnrollmentService{}, nil), nil)
	return app
}

// TestProgressBeaconRoute_RejectsForeignOrigin (MED-3, cap nhat N3 review vong 2 260915): request
// CO cookie accessToken (duong CSRF that su nham toi — xem N3 tai same_origin_middleware.go) tu
// mot origin la bi chan 403 TRUOC KHI cham auth/service — dung mutation de chung minh: go
// SameOriginRequired khoi enrollment_router.go se lam test nay do (request se roi thang xuong
// auth va tra 401 thay vi 403).
func TestProgressBeaconRoute_RejectsForeignOrigin(t *testing.T) {
	app := mountEnrollmentRoutesForOriginTest()

	req := httptest.NewRequest("POST", "/api/progress", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(&http.Cookie{Name: "accessToken", Value: "dummy-token-cho-test"})

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 (origin la + cookie accessToken phai bi chan truoc ca auth)", res.StatusCode)
	}
}

// TestProgressBeaconRoute_BearerKhongOrigin_BoQuaCSRFNhungVanCanAuthHopLe (N3, review vong 2
// 260915): client dung Bearer (khong cookie), khong dat Origin — truoc N3 se bi 403 OAN o
// SameOriginRequired; sau N3 phai VUOT QUA lop CSRF va toi duoc `auth`, roi bi 401 vi token gia
// (khong hop le) — chung minh CSRF layer khong con chan nham client Bearer nua, NHUNG van phai
// qua duoc kiem tra token that su o auth.
func TestProgressBeaconRoute_BearerKhongOrigin_BoQuaCSRFNhungVanCanAuthHopLe(t *testing.T) {
	app := mountEnrollmentRoutesForOriginTest()

	req := httptest.NewRequest("POST", "/api/progress", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Authorization", "Bearer gia-mao-khong-hop-le")
	// KHONG dat Origin, KHONG dat cookie.

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401 (token Bearer sai — nhung phai la 401 tu AuthMiddleware, "+
			"khong phai 403 tu SameOriginRequired, chung minh CSRF layer da bo qua request nay)", res.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(res.Body)
	if strings.Contains(string(bodyBytes), "request origin not allowed") {
		t.Fatalf("body chua thong bao cua SameOriginRequired — nghia la request bi chan CSRF thay vi toi duoc auth: %s", bodyBytes)
	}
}

// TestProgressBeaconRoute_RequiresAuth (MED-5): request tu origin HOP LE nhung KHONG kem token
// phai bi auth middleware chan 401 — Origin dung de tach rieng lop CSRF (MED-3) khoi lop auth
// (MED-5) dang kiem o day. Mutation: go `auth` khoi dong dang ky route trong
// enrollment_router.go se lam test nay do vi body loi doi tu "Missing access token" (cua chinh
// AuthMiddleware) sang "user ID not found in context" (fallback rieng cua handler qua
// extractUserID) — hai thong bao khac nhau nen assert theo NOI DUNG loi, khong chi theo status
// code 401 (status code giu nguyen 401 o ca hai truong hop do handler tu ve duoc, xem
// EnrollmentHandler.extractUserID).
func TestProgressBeaconRoute_RequiresAuth(t *testing.T) {
	app := mountEnrollmentRoutesForOriginTest()

	req := httptest.NewRequest("POST", "/api/progress", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Origin", testAllowedOrigin)

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401 (thieu token)", res.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(res.Body)
	body := string(bodyBytes)
	if !strings.Contains(body, "Missing access token") {
		t.Fatalf("body = %q, muon chua \"Missing access token\" — day la thong bao RIENG cua "+
			"AuthMiddleware; neu khong thay no nghia la request khong con di qua middleware auth "+
			"nua ma roi thang xuong handler", body)
	}
}
