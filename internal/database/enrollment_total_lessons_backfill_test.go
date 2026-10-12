package database

// Backfill enrollments.total_lessons kẹt ở 0 (Postgres thật, schema tạm). Mutation đã kiểm: bỏ `e.total_lessons = 0`
// (ghi đè tổng đã đúng), bỏ lọc is_mandatory / sections.deleted_at (đếm sai), hoặc bỏ runDataMigrationOnce (chạy lại
// mỗi lần khởi động) làm test ĐỎ.

import (
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestEnrollmentTotalLessonsBackfill_FixesStuckZeroTotals_RunsOnce(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	f := newLessonFixture(t, db)

	f.lesson("b1")
	f.lesson("b2")
	optional := f.lesson("không bắt buộc")
	if err := db.Exec("UPDATE lessons SET is_mandatory = false WHERE id = ?", optional).Error; err != nil {
		t.Fatalf("đặt bài không bắt buộc: %v", err)
	}
	deletedSec := model.Section{CourseID: f.courseID, Title: "Chương đã xoá", DisplayOrder: 2}
	if err := db.Create(&deletedSec).Error; err != nil {
		t.Fatalf("tạo chương: %v", err)
	}
	if err := db.Create(&model.Lesson{SectionID: deletedSec.ID, Title: "bài của chương đã xoá", DisplayOrder: 1}).Error; err != nil {
		t.Fatalf("tạo bài: %v", err)
	}
	if err := db.Delete(&model.Section{}, "id = ?", deletedSec.ID).Error; err != nil {
		t.Fatalf("xoá mềm chương: %v", err)
	}

	enroll := func(courseID uuid.UUID, total int) uuid.UUID {
		e := model.Enrollment{UserID: backfillUser(t, db, "en"), CourseID: courseID, TotalLessons: total}
		if err := db.Create(&e).Error; err != nil {
			t.Fatalf("tạo enrollment: %v", err)
		}
		return e.ID
	}
	emptyCourse := model.Course{InstructorID: backfillUser(t, db, "t"), Title: "QA-empty", Slug: "qa-empty-" + uuid.NewString()}
	if err := db.Create(&emptyCourse).Error; err != nil {
		t.Fatalf("tạo khoá trống: %v", err)
	}
	stuck := enroll(f.courseID, 0)
	softDeleted := enroll(f.courseID, 0)
	if err := db.Delete(&model.Enrollment{}, "id = ?", softDeleted).Error; err != nil {
		t.Fatalf("xoá mềm enrollment: %v", err)
	}
	alreadySet := enroll(f.courseID, 7) // số đã có (dù lệch) không được ghi đè: bước chỉ sửa dòng kẹt ở 0
	onEmptyCourse := enroll(emptyCourse.ID, 0)

	totalOf := func(id uuid.UUID) int {
		var e model.Enrollment
		if err := db.Unscoped().Where("id = ?", id).First(&e).Error; err != nil {
			t.Fatalf("đọc enrollment: %v", err)
		}
		return e.TotalLessons
	}

	// Migrate đã ghi dấu trên DB trống: xoá dấu để mô phỏng DB cũ khởi động lần đầu với bản mới.
	if err := db.Exec("DELETE FROM data_migrations WHERE name = ?", enrollmentTotalLessonsBackfillName).Error; err != nil {
		t.Fatalf("xoá dấu backfill: %v", err)
	}
	if err := runEnrollmentTotalLessonsBackfill(db); err != nil {
		t.Fatalf("backfill lần 1: %v", err)
	}
	for name, c := range map[string]struct {
		id   uuid.UUID
		want int
	}{
		"dòng kẹt ở 0":               {stuck, 2},
		"dòng đã xoá mềm":            {softDeleted, 2},
		"dòng đã có tổng (không đè)": {alreadySet, 7},
		"khoá chưa có bài":           {onEmptyCourse, 0},
	} {
		if got := totalOf(c.id); got != c.want {
			t.Errorf("%s: total_lessons = %d, muốn %d (chỉ đếm bài bắt buộc của chương chưa xoá)", name, got, c.want)
		}
	}

	// Chạy lại câu SQL: no-op (idempotent). Và bước chỉ chạy một lần: dòng kẹt mới sau đó không bị chạm nữa.
	if n, err := backfillEnrollmentTotalLessons(db); err != nil || n != 0 {
		t.Fatalf("câu SQL lần 2 sửa %d dòng (err=%v), muốn 0", n, err)
	}
	later := enroll(f.courseID, 0)
	if err := runEnrollmentTotalLessonsBackfill(db); err != nil {
		t.Fatalf("backfill lần 3: %v", err)
	}
	if got := totalOf(later); got != 0 {
		t.Errorf("bước đã ghi dấu không được chạy lại: dòng mới có total_lessons = %d, muốn 0", got)
	}
}

// Wired, not just present: RunPostMigrations (đường API khởi động chạy) phải gọi bước này.
func TestRunPostMigrations_RunsEnrollmentTotalLessonsBackfillOnOldDB(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	f := newLessonFixture(t, db)
	f.lesson("b1")
	e := model.Enrollment{UserID: backfillUser(t, db, "wire"), CourseID: f.courseID}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("tạo enrollment: %v", err)
	}
	if err := db.Exec("DELETE FROM data_migrations WHERE name = ?", enrollmentTotalLessonsBackfillName).Error; err != nil {
		t.Fatalf("xoá dấu backfill: %v", err)
	}
	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}
	var got model.Enrollment
	if err := db.Where("id = ?", e.ID).First(&got).Error; err != nil {
		t.Fatalf("đọc enrollment: %v", err)
	}
	if got.TotalLessons != 1 {
		t.Errorf("RunPostMigrations phải sửa total_lessons kẹt ở 0 -> 1, nhận %d", got.TotalLessons)
	}
}
