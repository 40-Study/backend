package repository

// Review đối kháng backend PR #70 (BLOCKER): RecalculateLessonStats thiếu
// "sections.deleted_at IS NULL" trong WHERE — SectionRepository.Delete là soft-delete (chỉ set
// deleted_at, không xoá hàng, nên FK OnDelete:CASCADE giữa sections->lessons không bao giờ kích
// hoạt), nên bài học của chương vừa xoá vẫn bị đếm và total_lessons/total_duration_minutes KHÔNG
// giảm sau khi xoá chương. Test mock hiện có (lesson_course_stats_test.go) chỉ kiểm wiring qua
// interface giả, không chạm câu SQL thật nên không bắt được lớp lỗi này — test dưới đây dùng
// đúng kỹ thuật DryRun+callback của enrollment_progress_lock_sql_test.go để đọc câu SQL thật mà
// RecalculateLessonStats sinh ra.
import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

func dryRunCourseRepo(t *testing.T) (*CourseRepository, *string) {
	t.Helper()
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	var captured string
	capture := func(d *gorm.DB) { captured = d.Statement.SQL.String() }
	// RecalculateLessonStats dùng db.Exec(...) (SQL thô) — Exec() chạy qua callbacks.Raw(),
	// KHÔNG qua callbacks.Query()/Create() như Find/Create thường dùng.
	if err := db.Callback().Raw().Register("test:capture_raw", capture); err != nil {
		t.Fatal(err)
	}
	return &CourseRepository{db: db.Session(&gorm.Session{DryRun: true})}, &captured
}

func TestRecalculateLessonStats_LocSectionsDaXoaMem(t *testing.T) {
	repo, sql := dryRunCourseRepo(t)
	_ = repo.RecalculateLessonStats(context.Background(), uuid.New())
	for _, want := range []string{"sections.deleted_at IS NULL"} {
		if !strings.Contains(*sql, want) {
			t.Fatalf("SQL thieu %q (bai hoc cua chuong da xoa mem van bi dem, total_lessons khong giam):\n%s", want, *sql)
		}
	}
	// Phai co ca 2 subquery (total_lessons va total_duration_minutes) deu duoc loc — dem so lan
	// xuat hien de tranh mot fix chi vi tri chua sua het.
	if got := strings.Count(*sql, "sections.deleted_at IS NULL"); got != 2 {
		t.Fatalf("muon ca 2 subquery (total_lessons + total_duration_minutes) co loc, duoc %d lan:\n%s", got, *sql)
	}
}
