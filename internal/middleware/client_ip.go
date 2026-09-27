package middleware

import (
	"net"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// TrustedProxySet (review vòng 2, PR #69): tập IP/CIDR "đáng tin" dùng cho ClientIPFromXFF —
// tách khỏi Fiber config (fiber.Config.TrustedProxies chỉ ảnh hưởng c.IP()/EnableIPValidation
// của chính Fiber, KHÔNG dùng được cho thuật toán "duyệt từ phải, bỏ qua proxy đáng tin" bên
// dưới vì Fiber chỉ hỗ trợ "IP đầu tiên hợp lệ", không hỗ trợ bỏ qua nhiều hop).
type TrustedProxySet struct {
	exact map[string]struct{}
	cidrs []*net.IPNet
}

// NewTrustedProxySet nhận danh sách chuỗi từ config.ResolvedTrustedProxies() (IP thường hoặc
// CIDR, ví dụ "127.0.0.1", "::1", "10.0.0.0/8").
func NewTrustedProxySet(entries []string) *TrustedProxySet {
	set := &TrustedProxySet{exact: make(map[string]struct{})}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			set.cidrs = append(set.cidrs, network)
			continue
		}
		set.exact[entry] = struct{}{}
	}
	return set
}

// Contains báo cáo ip có nằm trong tập đáng tin không. Nil-safe (set == nil hoặc rỗng -> không
// IP nào được coi là đáng tin) để mọi call site không cần tự kiểm tra nil trước khi gọi.
func (s *TrustedProxySet) Contains(ip string) bool {
	if s == nil || ip == "" {
		return false
	}
	if _, ok := s.exact[ip]; ok {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, network := range s.cidrs {
		if network.Contains(parsed) {
			return true
		}
	}
	return false
}

// ClientIPFromXFF (review vòng 2, PR #69 — MAJOR/BLOCKER bổ sung): fiber.Ctx.IP() với
// EnableIPValidation=true (cấu hình ở BuildFiberConfig, app.go) trả về IP ĐẦU TIÊN (bên trái
// nhất) trong X-Forwarded-For. Theo quy ước XFF, mỗi proxy chỉ NỐI THÊM IP nó quan sát được vào
// CUỐI chuỗi — nghĩa là các phần tử bên trái là do CLIENT tự khai (không proxy nào xác nhận), và
// một client thù địch hoàn toàn có thể tự đặt "X-Forwarded-For: 1.1.1.1, 2.2.2.2" để mỗi request
// rơi vào một bucket rate-limit khác nhau, né hoàn toàn giới hạn 5 lần/phút của /login.
//
// Giá trị ĐÁNG TIN là đầu BÊN PHẢI: duyệt xff từ phải sang trái, bỏ qua các phần tử là IP của
// chính một proxy trong TrustedProxies (proxy đó chỉ đang chuyển tiếp chuỗi cũ, không phải "IP
// client"); phần tử đầu tiên KHÔNG thuộc TrustedProxies chính là IP mà proxy đáng tin gần nhất
// (gần backend nhất) đã tự quan sát được khi nhận kết nối — không phải do client tự khai.
//
// Nếu TCP peer (kết nối trực tiếp tới backend) không đáng tin, bỏ qua toàn bộ XFF (ai cũng có
// thể tự gửi header này khi gọi thẳng backend), dùng thẳng peer.
func ClientIPFromXFF(peerIP string, xffHeader string, trusted *TrustedProxySet) string {
	if !trusted.Contains(peerIP) {
		return peerIP
	}
	if xffHeader == "" {
		return peerIP
	}
	parts := strings.Split(xffHeader, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		if !trusted.Contains(candidate) {
			return candidate
		}
	}
	// Toàn bộ chuỗi XFF chỉ toàn IP của các proxy đáng tin (không có client thật đứng sau, hiếm
	// gặp) -- fallback về peer thay vì trả chuỗi rỗng làm mọi request chia sẻ 1 bucket "".
	return peerIP
}

// ClientIP đọc TCP peer THẬT từ socket (fasthttp RemoteIP — độc lập với fiber.Config.ProxyHeader
// / EnableTrustedProxyCheck của chính Fiber, vì thuật toán bỏ-qua-nhiều-hop bên trên Fiber không
// tự hỗ trợ) và X-Forwarded-For thô, dùng cho MỌI rate limiter theo IP (S-P1-2 + BLOCKER, QA
// 260927) thay cho c.IP() mặc định.
//
// RÀNG BUỘC TRIỂN KHAI BẮT BUỘC (review vòng 3, PR #69): thuật toán này CHỈ an toàn khi TCP peer
// mà backend nhìn thấy KHÔNG THỂ là chính kẻ tấn công — nghĩa là backend không được lộ trực tiếp
// ra ngoài, chỉ nhận kết nối từ nginx (`nginx/nginx.conf`, đã đúng `$proxy_add_x_forwarded_for`).
// Nếu cổng backend (PORT, mặc định 5000) lộ thẳng ra ngoài (bind sai địa chỉ, firewall hỏng,
// tunnel debug quên tắt, ...), một client gọi thẳng có thể tự gửi `X-Forwarded-For` — Next.js
// (nếu Next là hop TRUNG GIAN) chỉ tự đặt `??= socket.remoteAddress` KHI header CHƯA có, nên vẫn
// giữ nguyên giá trị client tự khai nếu client gọi thẳng Next không qua nginx — client xoay XFF
// mỗi request để né hoàn toàn giới hạn theo IP dù thuật toán duyệt-từ-phải ở trên là đúng.
//
//  1. HOST (config.go, mặc định "localhost" — xem `Run()` trong app.go dùng
//     `fmt.Sprintf("%s:%s", Host, Port)` làm địa chỉ Listen) PHẢI giữ nguyên "localhost"/
//     "127.0.0.1" ở production, hoặc backend PHẢI đứng sau firewall/mạng nội bộ không cho kết
//     nối trực tiếp từ Internet vào PORT — chỉ nginx được phép gọi tới.
//  2. TRUSTED_PROXIES PHẢI liệt kê IP/CIDR của nginx (hop đứng NGAY TRƯỚC backend); nếu có thêm
//     một hop trung gian khác giữa nginx và backend (ví dụ Next.js tự gọi backend thay vì qua
//     nginx) thì liệt kê CẢ hop đó.
//  3. Giới hạn theo IP (AuthRateLimiter/WideAuthRateLimiter/OTPRateLimiter) là lớp phòng thủ
//     CHÍNH nhưng không phải DUY NHẤT — `AccountFailureLockout` (account_lockout.go) khoá theo
//     EMAIL, độc lập hoàn toàn với IP/XFF, vẫn chặn được brute-force ngay cả khi ràng buộc (1)/(2)
//     ở trên bị vi phạm do lỗi vận hành.
func ClientIP(c *fiber.Ctx, trusted *TrustedProxySet) string {
	peer := c.Context().RemoteIP().String()
	return ClientIPFromXFF(peer, c.Get(fiber.HeaderXForwardedFor), trusted)
}
