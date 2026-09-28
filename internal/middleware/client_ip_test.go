package middleware

// Review vòng 2 (plans/reports/review-260928-round2-integration.md), phần B: Fiber mặc định
// (EnableIPValidation, xem BuildFiberConfig) lấy IP ĐẦU TIÊN/bên trái nhất trong
// X-Forwarded-For — đó là giá trị CLIENT TỰ KHAI, một client thù địch có thể tự đặt bất kỳ chuỗi
// nào ở đó để mỗi request rơi vào 1 bucket rate-limit khác nhau, né hoàn toàn ngưỡng 5 lần/phút
// của /login. Test này khoá lại thuật toán ĐÚNG: duyệt XFF từ PHẢI sang trái, bỏ qua các phần tử
// là IP của chính một proxy đáng tin (TRUSTED_PROXIES), phần tử đầu tiên KHÔNG thuộc
// TRUSTED_PROXIES chính là IP proxy đáng tin gần nhất đã tự quan sát được; nếu TCP peer không
// đáng tin thì bỏ qua toàn bộ XFF, dùng thẳng peer.

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestClientIPFromXFF_PeerDuocTin_LayPhanTuNgoaiCungBenPhaiKhongThuocTrustedProxies(t *testing.T) {
	// Kịch bản coordinator nêu: peer = IP của Next (đáng tin), XFF = "spoof, 203.0.113.9" —
	// "spoof" là client tự khai (bên trái), "203.0.113.9" là IP Next thực sự quan sát được khi
	// nhận request (bên phải, do Next tự nối vào). Phải lấy 203.0.113.9, KHÔNG lấy "spoof".
	trusted := NewTrustedProxySet([]string{"10.0.0.5"}) // 10.0.0.5 = IP của Next
	got := ClientIPFromXFF("10.0.0.5", "spoof, 203.0.113.9", trusted)
	if got != "203.0.113.9" {
		t.Fatalf("client IP = %q, muon 203.0.113.9 (phan tu ngoai cung ben phai, khong thuoc TRUSTED_PROXIES)", got)
	}
}

func TestClientIPFromXFF_BoQuaNhieuHopProxyDangTinLienTiep(t *testing.T) {
	// Nhieu hop proxy dang tin cung noi IP cua chinh no vao XFF (khong chi 1 hop) — thuat toan
	// phai BO QUA het cac hop dang tin lien tiep tu phai sang, khong chi lay phan tu cuoi cung.
	trusted := NewTrustedProxySet([]string{"10.0.0.5", "10.0.0.6"}) // 2 hop proxy noi bo dang tin
	got := ClientIPFromXFF("10.0.0.6", "spoof, 203.0.113.9, 10.0.0.5, 10.0.0.6", trusted)
	if got != "203.0.113.9" {
		t.Fatalf("client IP = %q, muon 203.0.113.9 (bo qua ca 2 hop dang tin 10.0.0.5 va 10.0.0.6)", got)
	}
}

func TestClientIPFromXFF_PeerKhongDuocTin_BoQuaXFFHoanToan(t *testing.T) {
	// Peer KHONG nam trong TRUSTED_PROXIES (goi thang backend, khong qua Next) — bat ky ai cung
	// tu gui duoc header nay, nen phai bo qua hoan toan, dung thang peer. Hai gia tri XFF gia
	// mao khac nhau phai cho RA CUNG 1 client-ip-key (peer) — khong the tach bucket bang XFF.
	trusted := NewTrustedProxySet([]string{"10.0.0.5"}) // 10.0.0.5 = IP cua Next, KHONG phai peer nay
	peer := "203.0.113.50"

	got1 := ClientIPFromXFF(peer, "1.1.1.1", trusted)
	got2 := ClientIPFromXFF(peer, "2.2.2.2, 3.3.3.3", trusted)

	if got1 != peer || got2 != peer {
		t.Fatalf("peer khong dang tin phai luon tra ve peer (%q), bat ke XFF — got1=%q got2=%q", peer, got1, got2)
	}
}

func TestClientIPFromXFF_KhongCoXFF_DungPeer(t *testing.T) {
	trusted := NewTrustedProxySet([]string{"10.0.0.5"})
	got := ClientIPFromXFF("10.0.0.5", "", trusted)
	if got != "10.0.0.5" {
		t.Fatalf("khong co XFF -> phai dung peer, got %q", got)
	}
}

func TestClientIPFromXFF_TrustedNil_LuonDungPeer(t *testing.T) {
	// TRUSTED_PROXIES rong/khong cau hinh -> khong IP nao dang tin -> luon dung peer, an toan
	// theo mac dinh (fail-closed ve phia bo qua XFF, khong phai tin bua).
	got := ClientIPFromXFF("10.0.0.5", "1.1.1.1", nil)
	if got != "10.0.0.5" {
		t.Fatalf("trusted=nil phai luon dung peer, got %q", got)
	}
}

// TestClientIP_QuaFiberCtx_DocDungHeaderVaPeer: kiem tra day noi day that qua fiber.Ctx (khong
// chi ham thuan ClientIPFromXFF) — app.Test() cua Fiber luon gia lap TCP peer la "0.0.0.0" (xem
// fiber_config_test.go / helpers.go: testConn.RemoteAddr() hardcode 0.0.0.0:0), nen mo phong
// "peer dang tin" bang cach dua "0.0.0.0" vao TrustedProxies.
func TestClientIP_QuaFiberCtx_DocDungHeaderVaPeer(t *testing.T) {
	trusted := NewTrustedProxySet([]string{"0.0.0.0"})
	app := fiber.New()
	app.Get("/whoami", func(c *fiber.Ctx) error {
		return c.SendString(ClientIP(c, trusted))
	})

	req := httptest.NewRequest("GET", "/whoami", nil)
	req.Header.Set(fiber.HeaderXForwardedFor, "spoof, 203.0.113.9")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	got := string(buf[:n])
	if got != "203.0.113.9" {
		t.Fatalf("ClientIP qua fiber.Ctx = %q, muon 203.0.113.9", got)
	}
}
