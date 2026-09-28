package utils

import (
	"testing"
	"time"
)

// QA vòng 2 (G1): mốc đã biết 10:00 giờ Việt Nam ngày 28/09/2026 = 03:00 UTC.
func vnLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		// Windows/CI không có tzdata vẫn chạy được: Việt Nam cố định +07:00, không có giờ mùa hè.
		loc = time.FixedZone("ICT", 7*60*60)
	}
	return loc
}

func TestFormatTimestamp_GioVietNam_GiuDungOffset(t *testing.T) {
	local := time.Date(2026, 9, 28, 10, 0, 0, 0, vnLocation(t))

	got := FormatTimestamp(local)
	if got != "2026-09-28T10:00:00+07:00" {
		t.Fatalf("FormatTimestamp(10:00 giờ VN) = %q, muốn %q — chữ Z literal sẽ làm client đọc thành 17:00 giờ VN", got, "2026-09-28T10:00:00+07:00")
	}

	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("chuỗi %q không parse được bằng RFC3339: %v", got, err)
	}
	if !parsed.Equal(local) {
		t.Fatalf("parse lại %q ra %v, lệch khỏi mốc gốc %v", got, parsed.UTC(), local.UTC())
	}
}

func TestFormatTimestamp_GioUTC_InZ(t *testing.T) {
	utc := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC)
	if got := FormatTimestamp(utc); got != "2026-09-28T03:00:00Z" {
		t.Fatalf("FormatTimestamp(03:00 UTC) = %q, muốn 2026-09-28T03:00:00Z", got)
	}
}

func TestFormatTimestampPtr_NilVaCoGiaTri(t *testing.T) {
	if got := FormatTimestampPtr(nil); got != nil {
		t.Fatalf("FormatTimestampPtr(nil) = %q, muốn nil", *got)
	}
	local := time.Date(2026, 9, 28, 10, 0, 0, 0, vnLocation(t))
	got := FormatTimestampPtr(&local)
	if got == nil || *got != "2026-09-28T10:00:00+07:00" {
		t.Fatalf("FormatTimestampPtr(10:00 giờ VN) = %v, muốn 2026-09-28T10:00:00+07:00", got)
	}
}
