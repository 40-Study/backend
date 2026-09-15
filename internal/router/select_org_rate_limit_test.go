package router

// Test cho N5 (review vong 2, 260915): POST /api/auth/select-org bi bo sot authRateLimiter khi
// chuyen tu nhom cong khai sang nhom protected trong luc sua BLOCKER-1 — route nay cham
// Redis/DB va cap lai token giong het select-role (van con rate limiter), nen no la be mat khai
// thac giong M-03 da mo ta cho select-role.

import (
	"testing"
)

// TestSelectOrg_Live_RateLimited_SauNguongBiChan (N5): goi lien tiep tu CUNG mot IP (mac dinh
// cua httptest.NewRequest) vuot qua nguong AuthRateLimiter (5 lan/phut) — lan thu 6 phai bi 429,
// khong duoc xuong toi service nua. Mutation: bo `authRateLimiter` khoi dang ky route trong
// auth_router.go se lam test nay do (lan thu 6 se van la 409/200 tuy trang thai, khong bao gio
// la 429).
func TestSelectOrg_Live_RateLimited_SauNguongBiChan(t *testing.T) {
	env := newN2N6TestEnv(t, "STUDENT", nil, true)

	const max = 5
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
		t.Fatalf("lan goi thu %d, status = %d, muon 429 (Too Many Requests) — authRateLimiter phai duoc gan lai cho select-org, body: %s", max+1, status, raw)
	}
}
