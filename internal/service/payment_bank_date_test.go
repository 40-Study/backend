package service

// Review #76 vòng 3 MINOR 5 (mutation B5 sống sót): test Postgres dựng ngày giao dịch bằng chính
// bankTimeZone nên đổi múi giờ ngân hàng sang UTC vẫn xanh. Test này dùng CHUỖI CỐ ĐỊNH theo đúng
// định dạng mbbank (giờ Việt Nam, không kèm múi giờ) và mốc UTC mong đợi viết tay.

import (
	"testing"
	"time"
)

func TestParseBankTransactionDate_IsVietnamTime(t *testing.T) {
	cases := map[string]time.Time{
		"28/09/2026 07:00:00": time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		"28/09/2026 06:59:59": time.Date(2026, 9, 27, 23, 59, 59, 0, time.UTC),
		"01/10/2026 00:30":    time.Date(2026, 9, 30, 17, 30, 0, 0, time.UTC),
		"2026-09-28 07:00:00": time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		// Có múi giờ tường minh thì theo đúng múi giờ đó.
		"2026-09-28T07:00:00Z": time.Date(2026, 9, 28, 7, 0, 0, 0, time.UTC),
	}
	for in, want := range cases {
		got, ok := parseBankTransactionDate(in)
		if !ok || !got.Equal(want) {
			t.Errorf("parseBankTransactionDate(%q) = %s ok=%t, muốn %s", in, got.UTC(), ok, want)
		}
	}
	for _, bad := range []string{"", "   ", "hôm qua", "28-09-2026"} {
		if _, ok := parseBankTransactionDate(bad); ok {
			t.Errorf("parseBankTransactionDate(%q) ok=true, muốn false (không coi là trong hạn)", bad)
		}
	}
}
