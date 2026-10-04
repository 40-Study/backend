package seeds

// QA-reg A-22: hạn nộp bài demo "23:59" hiện là 06:59 hôm sau vì seed dùng time.Local. Production là container
// `FROM scratch` (time.Local = UTC), máy dev Windows là VN: seed phải ra CÙNG khoảnh khắc ở cả hai. Test đặt
// time.Local = UTC để mô phỏng production; đổi seedZone về time.Local thì ĐỎ.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"study.com/v1/internal/model"
)

func withLocalUTC(t *testing.T) {
	t.Helper()
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
}

func TestDemoDueAt_Is2359VietnamTimeRegardlessOfProcessZone(t *testing.T) {
	withLocalUTC(t)
	today := demoTodayAt(time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC)) // 10:00 ngày 04/10 giờ VN
	due := demoWallClock(today, 1, 23, 59)

	if got := due.In(seedZone).Format("2006-01-02 15:04"); got != "2026-10-05 23:59" {
		t.Fatalf("hạn nộp theo giờ VN = %s, muốn 2026-10-05 23:59", got)
	}
	// 23:59 +07 = 16:59 UTC: nếu seed dùng giờ máy chủ (UTC) thì ra 23:59 UTC = 06:59 VN hôm sau.
	if got := due.UTC().Format("15:04"); got != "16:59" {
		t.Fatalf("hạn nộp theo UTC = %s, muốn 16:59 (23:59 giờ VN)", got)
	}
}

func TestDemoToday_UsesVietnamCalendarDay(t *testing.T) {
	withLocalUTC(t)
	// 20:00 UTC ngày 04/10 đã là 03:00 ngày 05/10 ở Việt Nam.
	today := demoTodayAt(time.Date(2026, 10, 4, 20, 0, 0, 0, time.UTC))
	if got := today.Format("2006-01-02 15:04"); got != "2026-10-05 00:00" {
		t.Fatalf("hôm nay theo giờ VN = %s, muốn 2026-10-05 00:00", got)
	}
}

func TestAtClock_UsesVietnamTime(t *testing.T) {
	withLocalUTC(t)
	day := demoTodayAt(time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC))
	got, err := atClock(day, model.TimeOfDay("19:00"))
	if err != nil {
		t.Fatal(err)
	}
	if got.UTC().Format("15:04") != "12:00" {
		t.Fatalf("19:00 giờ VN = %s UTC, muốn 12:00", got.UTC().Format("15:04"))
	}
}

// Giờ đồng hồ của seed phải đi qua seedZone. Quét mã nguồn thay vì chạy: máy dev VN và container UTC cho cùng kết
// quả với time.Local ở máy dev, nên chỉ có quét mã mới bắt được một chỗ lại dùng time.Local.
func TestSeeds_NoTimeLocalInSeedSources(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("không đọc được nguồn seeds: %v (%d file)", err, len(files))
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "time.Local") {
			t.Errorf("%s dùng time.Local: dùng seedZone (Asia/Ho_Chi_Minh) để hạn nộp/giờ phiên không lệch theo máy chạy seed (A-22)", f)
		}
	}
}
