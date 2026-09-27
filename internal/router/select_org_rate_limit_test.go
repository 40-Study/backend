package router

// Test cho N5 (review vong 2, 260915): POST /api/auth/select-org bi bo sot rate-limit khi
// chuyen tu nhom cong khai sang nhom protected trong luc sua BLOCKER-1.
//
// S-P1-2 (QA 260927, ban va bo sung): nguong da doi tu 5/phut (authRateLimiter, dung chung voi
// /login) sang 30/phut (postAuthRateLimiter, bucket "rate:post-auth" dung chung voi select-role
// va refresh-token) — vi select-org dung SAU AuthMiddleware (bat buoc access token that con hieu
// luc) nen khong phai be mat do mat khau, va dung chung bucket chat voi /login se tu chan nguoi
// dung doi to chuc nhieu lan trong 1 phien lam viec that.

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

// postSelectRoleEmpty (S-P1-2, QA 260927 — bổ sung): /select-role nằm ở nhóm CÔNG KHAI (trước
// AuthMiddleware, dùng session_token của luồng đăng nhập — xem M-03), khác với /select-org nên
// KHÔNG gắn Authorization header. Không quan tâm kết quả nghiệp vụ (400/401 vì thiếu
// session_token thật) — chỉ cần biết rate limiter có chặn ở đúng ngưỡng hay không.
func (e *n2n6TestEnv) postSelectRoleEmpty(t *testing.T) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/auth/select-role", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// TestSelectOrg_Live_RateLimited_SauNguongBiChan (N5, cap nhat nguong theo S-P1-2): goi lien tiep
// tu CUNG mot IP vuot qua nguong postAuthRateLimiter (30 lan/phut) — lan thu 31 phai bi 429,
// khong duoc xuong toi service nua. Mutation: bo rate limiter khoi dang ky route select-org trong
// auth_router.go, HOAC gan nham lai authRateLimiter (5/phut) cho no, deu lam test nay do (khong
// con dung 429 o dung lan thu 31).
func TestSelectOrg_Live_RateLimited_SauNguongBiChan(t *testing.T) {
	env := newN2N6TestEnv(t, "STUDENT", nil, true)

	const max = 30
	for i := 0; i < max; i++ {
		status, raw := env.postSelectOrgEmpty(t)
		// Khong quan tam ket qua nghiep vu (co the 200/409 tuy fixture) — chi can KHONG phai 429
		// trong nguong cho phep.
		if status == 429 {
			t.Fatalf("lan goi thu %d (trong nguong %d) da bi 429 — nguong rate limit qua chat hoac loi cau hinh, body: %s", i+1, max, raw)
		}
	}

	status, raw := env.postSelectOrgEmpty(t)
	if status != 429 {
		t.Fatalf("lan goi thu %d, status = %d, muon 429 (Too Many Requests) — postAuthRateLimiter phai duoc gan cho select-org, body: %s", max+1, status, raw)
	}
}

// TestSelectRole_Live_KhongTuChanChinhMinhBangBucketCuaLogin (S-P1-2, QA 260927 — bổ sung): kiểm
// tra TRÊN APP THẬT (SetupAuthRoutes thật, không phải mô phỏng ở middleware) rằng /select-role
// dùng bucket postAuthRateLimiter (30/phút), KHÔNG còn dùng chung authRateLimiter (5/phút) với
// /login. Đây chính là khoảng trống mà một test chỉ mô phỏng ở tầng middleware (không đi qua
// auth_router.go thật) sẽ KHÔNG bắt được — xác nhận bằng mutation: gán lại `authRateLimiter` cho
// dòng `/select-role` trong auth_router.go làm đúng test này đỏ ở lần gọi thứ 6 (không phải 31).
func TestSelectRole_Live_KhongTuChanChinhMinhBangBucketCuaLogin(t *testing.T) {
	env := newN2N6TestEnv(t, "STUDENT", nil, true)

	const max = 30
	for i := 0; i < max; i++ {
		status, raw := env.postSelectRoleEmpty(t)
		if status == 429 {
			t.Fatalf("lan goi thu %d (trong nguong %d) da bi 429 — select-role dang dung nham bucket chat (authRateLimiter), body: %s", i+1, max, raw)
		}
	}
	status, raw := env.postSelectRoleEmpty(t)
	if status != 429 {
		t.Fatalf("lan goi thu %d, status = %d, muon 429 — postAuthRateLimiter phai duoc gan cho select-role, body: %s", max+1, status, raw)
	}
}
