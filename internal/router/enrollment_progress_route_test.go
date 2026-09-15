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
	SetupEnrollmentRoutes(api, cfg, handler.NewEnrollmentHandler(&fakeEnrollmentService{}), nil)
	return app
}

// TestProgressBeaconRoute_RejectsForeignOrigin (MED-3): request khong Origin/Referer hop le bi
// chan 403 TRUOC KHI cham auth/service — dung mutation de chung minh: go SameOriginRequired
// khoi enrollment_router.go se lam test nay do (request se roi thang xuong auth va tra 401 thay
// vi 403).
func TestProgressBeaconRoute_RejectsForeignOrigin(t *testing.T) {
	app := mountEnrollmentRoutesForOriginTest()

	req := httptest.NewRequest("POST", "/api/progress", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Origin", "https://evil.example")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 (origin la phai bi chan truoc ca auth)", res.StatusCode)
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
