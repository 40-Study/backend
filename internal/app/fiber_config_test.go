package app

// Test cho BLOCKER trong review PR #69 (QA 260927): c.IP() phải đọc X-Forwarded-For CHỈ khi TCP
// peer nằm trong TrustedProxies, và trả về IP client thật (không phải nguyên chuỗi header) khi
// được phép đọc.
//
// Giới hạn kỹ thuật đã xác nhận bằng cách đọc thẳng mã nguồn fiber@v2.52.12 (helpers.go#testConn):
// `app.Test()` dựng request từ `httputil.DumpRequest` rồi feed qua một `net.Conn` giả
// (`testConn`) có `RemoteAddr()` LUÔN CỐ ĐỊNH là `0.0.0.0:0` — `req.RemoteAddr` do test tự đặt
// KHÔNG được dùng (RemoteAddr là thuộc tính kết nối, không nằm trong bytes HTTP thô được dump).
// Vì vậy "TCP peer" trong MỌI test ở file này luôn là "0.0.0.0" — test mô phỏng "peer được tin"
// bằng cách đưa "0.0.0.0" vào TrustedProxies, và "peer không được tin" bằng cách KHÔNG đưa vào.

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/config"
)

// TestBuildFiberConfig_DocDungCacTruongTuConfig (mutation): xoa bat ky truong nao trong
// BuildFiberConfig (fiber_config.go) se lam test nay do — dam bao khong ai vo tinh xoa
// EnableTrustedProxyCheck/EnableIPValidation/ProxyHeader trong mot lan sua sau nay.
func TestBuildFiberConfig_DocDungCacTruongTuConfig(t *testing.T) {
	cfg := &config.Config{TrustedProxies: "1.2.3.4,5.6.7.8"}
	fc := BuildFiberConfig(cfg)

	if fc.ProxyHeader != fiber.HeaderXForwardedFor {
		t.Errorf("ProxyHeader = %q, muon %q", fc.ProxyHeader, fiber.HeaderXForwardedFor)
	}
	if !fc.EnableTrustedProxyCheck {
		t.Error("EnableTrustedProxyCheck = false, muon true — thieu no thi bat ky client nao cung gia mao duoc X-Forwarded-For")
	}
	if !fc.EnableIPValidation {
		t.Error("EnableIPValidation = false, muon true — thieu no thi c.IP() tra nguyen chuoi header tho thay vi 1 IP hop le")
	}
	want := []string{"1.2.3.4", "5.6.7.8"}
	if len(fc.TrustedProxies) != len(want) || fc.TrustedProxies[0] != want[0] || fc.TrustedProxies[1] != want[1] {
		t.Errorf("TrustedProxies = %v, muon %v", fc.TrustedProxies, want)
	}
}

// TestBuildFiberConfig_MacDinhKhiTrustedProxiesRong: TRUSTED_PROXIES rong -> fallback
// "127.0.0.1,::1" (config.DefaultTrustedProxies), khop dung dev/localhost.
func TestBuildFiberConfig_MacDinhKhiTrustedProxiesRong(t *testing.T) {
	fc := BuildFiberConfig(&config.Config{})
	want := []string{"127.0.0.1", "::1"}
	if len(fc.TrustedProxies) != len(want) || fc.TrustedProxies[0] != want[0] || fc.TrustedProxies[1] != want[1] {
		t.Errorf("TrustedProxies (mac dinh) = %v, muon %v", fc.TrustedProxies, want)
	}
}

// whoamiApp dung 1 route /whoami tra ve c.IP() de kiem tra hanh vi RUNTIME THAT (khong chi doc
// gia tri config) voi cau hinh trustedProxies tuy chinh.
func whoamiApp(trustedProxies []string) *fiber.App {
	app := fiber.New(fiber.Config{
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		EnableIPValidation:      true,
		TrustedProxies:          trustedProxies,
	})
	app.Get("/whoami", func(c *fiber.Ctx) error { return c.SendString(c.IP()) })
	return app
}

func whoami(t *testing.T, app *fiber.App, xff string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/whoami", nil)
	if xff != "" {
		req.Header.Set(fiber.HeaderXForwardedFor, xff)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

// TestClientIP_PeerDuocTin_XFFPhanBietDuocTungNguoiDung (test a, review PR #69): peer (luon
// "0.0.0.0" trong test — xem comment dau file) nam trong TrustedProxies -> c.IP() phai doc
// X-Forwarded-For va tra ve DUNG IP client (KHONG phai "0.0.0.0", KHONG phai nguyen chuoi header
// neu header co nhieu gia tri) — 2 client khac nhau (XFF khac nhau) phai cho ra 2 IP khac nhau.
func TestClientIP_PeerDuocTin_XFFPhanBietDuocTungNguoiDung(t *testing.T) {
	app := whoamiApp([]string{"0.0.0.0"})

	ipA := whoami(t, app, "203.0.113.10")
	ipB := whoami(t, app, "203.0.113.20")

	if ipA == "0.0.0.0" || ipB == "0.0.0.0" {
		t.Fatalf("c.IP() tra ve TCP peer (0.0.0.0) thay vi doc X-Forwarded-For du peer nam trong TrustedProxies: ipA=%q ipB=%q", ipA, ipB)
	}
	if ipA == ipB {
		t.Fatalf("2 client voi X-Forwarded-For KHAC NHAU (%q vs %q) lai cho ra CUNG 1 c.IP() = %q — khong phan biet duoc nguoi dung, dung nguyen nhan bug S-P1-2/BLOCKER", "203.0.113.10", "203.0.113.20", ipA)
	}
	if ipA != "203.0.113.10" {
		t.Errorf("ipA = %q, muon dung \"203.0.113.10\" (IP DAU TIEN trong chuoi XFF, theo quy uoc la client that)", ipA)
	}
	if ipB != "203.0.113.20" {
		t.Errorf("ipB = %q, muon dung \"203.0.113.20\"", ipB)
	}

	// Chuoi XFF nhieu gia tri (client that + cac hop truoc do, giong dung contract proxy Next.js
	// noi them IP vao CUOI chuoi cu) van phai tra dung IP DAU TIEN (client that), khong phai
	// nguyen chuoi.
	ipMulti := whoami(t, app, "198.51.100.5, 10.0.0.1")
	if ipMulti != "198.51.100.5" {
		t.Errorf("XFF nhieu gia tri: c.IP() = %q, muon \"198.51.100.5\" (gia tri DAU TIEN) — neu tra nguyen chuoi thi EnableIPValidation da bi thieu", ipMulti)
	}
}

// TestClientIP_PeerKhongTinCay_BoQuaXFFGiaMao (test b, review PR #69): peer KHONG nam trong
// TrustedProxies -> c.IP() PHAI bo qua X-Forwarded-For (du client tu xung IP gia) va luon tra ve
// TCP peer that — 2 request voi XFF GIA khac nhau phai cho ra CUNG 1 IP (peer that, khong phai
// gia tri client tu xung).
func TestClientIP_PeerKhongTinCay_BoQuaXFFGiaMao(t *testing.T) {
	// KHONG dua "0.0.0.0" (peer that trong test) vao danh sach — mo phong "day khong phai hop
	// proxy tin cay".
	app := whoamiApp([]string{"203.0.113.99"})

	ipFake1 := whoami(t, app, "198.51.100.1")
	ipFake2 := whoami(t, app, "198.51.100.2")

	if ipFake1 != ipFake2 {
		t.Fatalf("peer KHONG duoc tin nhung 2 XFF gia mao khac nhau (%q vs %q) van cho ra 2 IP khac nhau (%q vs %q) — nghia la header van duoc doc du peer khong tin cay, mo cua cho gia mao IP/ne rate-limit", "198.51.100.1", "198.51.100.2", ipFake1, ipFake2)
	}
	if ipFake1 == "198.51.100.1" || ipFake1 == "198.51.100.2" {
		t.Fatalf("c.IP() = %q — da doc X-Forwarded-For gia mao du peer KHONG nam trong TrustedProxies (chi co the lay tu header, khong phai TCP peer That)", ipFake1)
	}
}
