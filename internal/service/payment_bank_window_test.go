package service

// Review #76 final, QUYẾT ĐỊNH 2 (MAJOR 2, probe P3): mốc ngày gửi sang gRPC tính theo giờ VN, không
// phụ thuộc múi giờ server gRPC (chưa triển khai; image python:3.11-slim mặc định UTC). Test thuần,
// không cần DB.

import (
	"errors"
	"testing"
	"time"

	"study.com/v1/internal/model"
)

func windowOrder(deadlineUTC time.Time) *model.Order {
	code := "PAYQA-WINDOW"
	d := deadlineUTC
	return &model.Order{PaymentTransactionID: &code, PaymentCodeExpiredAt: &d, CreatedAt: d.Add(-23 * time.Hour)}
}

func TestBankLookupWindow_EndOfVietnamDayAtVNMidnight(t *testing.T) {
	cases := []struct {
		name    string
		now     time.Time
		wantDay string // ngày (giờ VN) của to; ngày UTC của to phải trùng
	}{
		// 00:30 giờ VN ngày 29/09 = 17:30 UTC ngày 28/09. Dùng now thì server UTC tra tới 28/09 và bỏ
		// mất giao dịch 00:00–00:30 ngày 29/09.
		{"00:30 giờ VN (17:30 UTC hôm trước)", time.Date(2026, 9, 28, 17, 30, 0, 0, time.UTC), "2026-09-29"},
		// Probe P3: hạn mã 03:00 VN 29/09, kiểm lúc 03:31 VN, tiền về lúc 01:00 VN 29/09.
		{"probe P3: 03:31 giờ VN", time.Date(2026, 9, 28, 20, 31, 0, 0, time.UTC), "2026-09-29"},
		{"23:59 giờ VN", time.Date(2026, 9, 29, 16, 59, 0, 0, time.UTC), "2026-09-29"},
	}
	deadline := time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC) // 03:00 VN 29/09
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, to := bankLookupWindow(windowOrder(deadline), c.now)
			vn := bankTimeZone()
			if got := to.In(vn); got.Format("2006-01-02 15:04:05") != c.wantDay+" 23:59:59" {
				t.Fatalf("to (giờ VN) = %s, muốn %s 23:59:59", got, c.wantDay)
			}
			if got := to.UTC().Format("2006-01-02"); got != c.wantDay {
				t.Fatalf("to (UTC) = %s thuộc ngày %s, muốn %s: server gRPC chạy UTC sẽ tra lệch ngày", to.UTC(), got, c.wantDay)
			}
			paidAt := time.Date(2026, 9, 29, 1, 0, 0, 0, vn) // 01:00 VN 29/09
			if to.Before(paidAt) {
				t.Fatalf("to = %s trước giao dịch lúc %s: tiền trong hạn bị bỏ sót", to, paidAt)
			}
		})
	}
}

// Cửa sổ cố định: đơn cũ không làm to chạy theo hôm nay, from là đầu ngày VN.
func TestBankLookupWindow_FixedForOldOrders(t *testing.T) {
	deadline := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC) // 03:00 VN 02/09
	from, to := bankLookupWindow(windowOrder(deadline), time.Date(2026, 9, 28, 5, 0, 0, 0, time.UTC))
	vn := bankTimeZone()
	if got := to.In(vn).Format("2006-01-02 15:04:05"); got != "2026-09-03 23:59:59" {
		t.Fatalf("to = %s, muốn cuối ngày VN của hạn mã + 30 phút + 1 ngày (2026-09-03 23:59:59)", got)
	}
	// created_at = hạn mã - 23h = 04:00 VN 01/09 → from = đầu ngày VN 31/08.
	if got := from.In(vn).Format("2006-01-02 15:04:05"); got != "2026-08-31 00:00:00" {
		t.Fatalf("from = %s, muốn 2026-08-31 00:00:00 giờ VN", got)
	}
}

// Máy thiếu tzdata: không lỗi, fallback +07.
func TestResolveBankTimeZone_FallsBackWithoutTzdata(t *testing.T) {
	loc := resolveBankTimeZone(func(string) (*time.Location, error) { return nil, errors.New("unknown time zone Asia/Ho_Chi_Minh") })
	if _, off := time.Date(2026, 9, 29, 0, 0, 0, 0, loc).Zone(); off != 7*60*60 {
		t.Fatalf("offset fallback = %d, muốn +07:00", off)
	}
	loaded := resolveBankTimeZone(func(name string) (*time.Location, error) { return time.FixedZone(name, 7*60*60), nil })
	if loaded.String() != bankTimeZoneName {
		t.Fatalf("khi nạp được thì dùng %s, được %s", bankTimeZoneName, loaded)
	}
}
