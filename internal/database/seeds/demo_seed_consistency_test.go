package seeds

// QA 10/11 (F6): dữ liệu demo tự mâu thuẫn. Test thuần (không cần DB) cho các hàm sinh dữ liệu:
//   - QA-A-04: chuỗi ngày và huy hiệu suy ra từ lịch sử học thật.
//   - QA-B-15: giờ lịch cá nhân theo giờ Việt Nam dù process chạy ở UTC.
//   - QA-B-12: buổi học không vượt ngày kết thúc lớp.
// QA-A-03 (điểm bài thi = tổng điểm đáp án) ở demo_contest_answers_test.go.

import (
	"testing"
	"time"

	"study.com/v1/internal/model"
)

func TestDemoSessionLastDay_StopsAtClassEndDate(t *testing.T) {
	today := time.Date(2026, 11, 10, 0, 0, 0, 0, seedZone)
	// Lớp Flutter K5 kết thúc 04/11 (cột date đọc về là 00:00 UTC): không được sinh "Buổi 12" ngày 14/11.
	ended := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	if got := demoSessionLastDay(today, &ended); got.Format("2006-01-02") != "2026-11-04" {
		t.Fatalf("lớp đã kết thúc 04/11: ngày cuối sinh buổi = %s, muốn 2026-11-04", got.Format("2006-01-02"))
	}
	far := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, end := range []*time.Time{&far, nil} {
		if got := demoSessionLastDay(today, end); got.Format("2006-01-02") != "2026-11-24" {
			t.Errorf("lớp còn hạn: ngày cuối = %s, muốn hôm nay + %d = 2026-11-24", got.Format("2006-01-02"), demoSessionLookaheadDays)
		}
	}
}

func TestCalendarSlot_UsesVietnamTime(t *testing.T) {
	withLocalUTC(t)
	now := time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC) // 10:00 ngày 04/10 giờ VN
	got := calendarSlot(now, 1, 19)
	if v := got.In(seedZone).Format("2006-01-02 15:04"); v != "2026-10-05 19:00" {
		t.Fatalf("sự kiện 19:00 ngày mai theo giờ VN = %s, muốn 2026-10-05 19:00", v)
	}
	if u := got.UTC().Format("15:04"); u != "12:00" {
		t.Fatalf("19:00 giờ VN = %s UTC, muốn 12:00 (seed dùng giờ máy UTC thì ra 19:00 UTC = 02:00 VN)", u)
	}
}

func TestDemoCompletionTime_NeverInFutureAndSameCalendarDayInUTC(t *testing.T) {
	morning := time.Date(2026, 11, 10, 8, 0, 0, 0, seedZone) // trước 09:00 VN
	if got := demoCompletionTime(morning, 0); got.Format("2006-01-02 15:04") != "2026-11-09 09:00" {
		t.Errorf("08:00 VN: mốc gần nhất = %s, muốn 09:00 hôm qua (không ghi mốc tương lai)", got.Format("2006-01-02 15:04"))
	}
	noon := time.Date(2026, 11, 10, 12, 0, 0, 0, seedZone)
	got := demoCompletionTime(noon, 2)
	if got.Format("2006-01-02 15:04") != "2026-11-08 09:00" {
		t.Errorf("12:00 VN, lùi 2 ngày = %s, muốn 2026-11-08 09:00", got.Format("2006-01-02 15:04"))
	}
	// Backend nhóm hoạt động theo DATE(completed_at) ở UTC: ngày UTC phải trùng ngày VN.
	if got.UTC().Format("2006-01-02") != got.Format("2006-01-02") {
		t.Errorf("ngày UTC %s khác ngày VN %s", got.UTC().Format("2006-01-02"), got.Format("2006-01-02"))
	}
}

func TestDemoStreaks_DerivedFromActivity(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, seedZone)
	at := func(daysBefore int) time.Time { return demoCompletionTime(now, daysBefore) }
	cases := []struct {
		name          string
		times         []time.Time
		current, long int
	}{
		{"không học gì", nil, 0, 0},
		{"3 ngày liền tới hôm nay", []time.Time{at(2), at(1), at(0)}, 3, 3},
		{"chuỗi tới hôm qua vẫn còn", []time.Time{at(2), at(1)}, 2, 2},
		// QA-A-04: hoạt động cuối từ hơn 7 ngày trước thì chuỗi hiện tại phải là 0, không phải 3.
		{"học xong từ 9 ngày trước", []time.Time{at(11), at(10), at(9)}, 0, 3},
		{"gãy chuỗi, nhiều bài cùng ngày", []time.Time{at(6), at(5), at(5), at(4), at(4), at(1), at(0)}, 2, 3},
	}
	for _, tc := range cases {
		if c, l := demoStreaks(now, tc.times); c != tc.current || l != tc.long {
			t.Errorf("%s: chuỗi (%d hiện tại, %d dài nhất), muốn (%d, %d)", tc.name, c, l, tc.current, tc.long)
		}
	}
}

func demoAchievementBySlug(t *testing.T, slug string) model.Achievement {
	t.Helper()
	for _, a := range demoAchievements {
		if a.Slug == slug {
			return a
		}
	}
	t.Fatalf("không có thành tích %s", slug)
	return model.Achievement{}
}

func TestDemoAchievementEarnedAt_OnlyWhenRequirementMet(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, seedZone)
	five := []time.Time{}
	for d := 4; d >= 0; d-- {
		five = append(five, demoCompletionTime(now, d))
	}
	cases := []struct {
		slug string
		f    demoLearnerFacts
		want bool
	}{
		// QA-A-04: 0 bài hoàn thành thì không có "Bước chân đầu tiên".
		{"buoc-chan-dau-tien", demoLearnerFacts{}, false},
		{"buoc-chan-dau-tien", demoLearnerFacts{Lessons: five[:1]}, true},
		{"hoc-vien-cham-chi", demoLearnerFacts{Lessons: five}, false},
		{"chuoi-7-ngay", demoLearnerFacts{Lessons: five}, false},
		{"hoan-thanh-khoa-dau-tien", demoLearnerFacts{Courses: five[:1]}, true},
		{"nguoi-danh-gia", demoLearnerFacts{}, false},
		{"nguoi-danh-gia", demoLearnerFacts{Reviews: five[:1]}, true},
		{"bac-thay-trac-nghiem", demoLearnerFacts{PassedQuizzes: five[:4]}, false},
		// Seed không có dữ liệu giờ học nên không cấp "Cú đêm" / thành tích bí mật dù có nhiều hoạt động.
		{"cu-dem", demoLearnerFacts{Lessons: five}, false},
		{"bi-mat-40study", demoLearnerFacts{Lessons: five}, false},
	}
	for _, tc := range cases {
		if _, got := demoAchievementEarnedAt(demoAchievementBySlug(t, tc.slug), tc.f); got != tc.want {
			t.Errorf("%s với %d bài/%d khoá/%d đánh giá/%d quiz: đạt=%v, muốn %v", tc.slug, len(tc.f.Lessons),
				len(tc.f.Courses), len(tc.f.Reviews), len(tc.f.PassedQuizzes), got, tc.want)
		}
	}
	if at, ok := demoAchievementEarnedAt(demoAchievementBySlug(t, "chuoi-7-ngay"), demoLearnerFacts{Lessons: append(
		[]time.Time{demoCompletionTime(now, 6), demoCompletionTime(now, 5)}, five...)}); !ok || !at.Equal(five[4]) {
		t.Errorf("7 ngày liền: đạt=%v lúc %v, muốn đạt lúc %v (ngày thứ 7)", ok, at, five[4])
	}
}

// Dữ liệu demo của student2 (người QA dùng) phải tự nhất quán: số bài hoàn thành của khoá Python đủ tạo chuỗi ngày và
// huy hiệu bài học đầu tiên. Đổi lại Progress 10% (0/6 bài) thì ĐỎ.
func TestDemoStudent2_LessonsBackStreakAndFirstLessonBadge(t *testing.T) {
	var spec *enrollmentSpec
	for i := range demoEnrollments {
		if demoEnrollments[i].StudentEmail == "student2@demo.com" {
			spec = &demoEnrollments[i]
		}
	}
	if spec == nil {
		t.Fatal("student2 không có ghi danh demo")
	}
	lessons := -1
	for _, c := range demoCourses {
		if c.Slug == spec.CourseSlug {
			lessons = totalLessons(c.Sections)
		}
	}
	n := demoCompletedLessonCount(lessons, spec.Progress)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, seedZone)
	var times []time.Time
	for i := 0; i < n; i++ {
		times = append(times, demoCompletionTime(now, n-1-i))
	}
	current, _ := demoStreaks(now, times)
	_, badge := demoAchievementEarnedAt(demoAchievementBySlug(t, "buoc-chan-dau-tien"), demoLearnerFacts{Lessons: times})
	if n < 3 || current != n || !badge {
		t.Fatalf("student2: %d/%d bài hoàn thành, chuỗi %d ngày, huy hiệu bài đầu=%v; muốn ≥3 bài, chuỗi = số bài, có huy hiệu",
			n, lessons, current, badge)
	}
}
