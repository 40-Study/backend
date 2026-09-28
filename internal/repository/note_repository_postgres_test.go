package repository

// R5 (code-reviewer-260919-1557): "column reference \"created_at\" is ambiguous" CHI xuat hien
// khi Postgres THAT SU phan giai cau lenh (ListByCourse JOIN "lessons" khi co sectionID, va
// bang do CUNG co cot created_at) — mot repo gia lap (mock/fake) khong bao gio bat duoc loi
// nay, vi lo hong nam trong CHINH cau SQL. Test nay ket noi Postgres THAT (khong mock), tao du
// lieu rieng trong MOT transaction roi ROLLBACK, va bo qua (t.Skip) neu khong ket noi duoc — day
// la test tich hop bo sung, khong thay the cac test dry-run/mock o noi khac.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

// openTestPostgres mo ket noi Postgres THAT qua pgtest.Open: bo qua khi chay local khong co DB,
// nhung FAIL khi CI=true (review Phase 4, B-2). Cac test dung ham nay deu tu ROLLBACK.
func openTestPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	return pgtest.Open(t)
}

// TestListByCourse_LocTheoSection_KhongLoiAmbiguousColumn (R5): tai san sinh CHINH XAC kich ban
// bao cao — hoc vien mo tab "Trong chuong hien tai" (web goi ListByCourse voi sectionID khac
// nil). Truoc ban va nay, ORDER BY "created_at DESC" khong co tien to bang trong khi cau lenh co
// JOIN "lessons" (cung co cot created_at) khien Postgres tra loi 42702 (ambiguous), ListByCourse
// tra ve error, handler bien no thanh 500. Sau ban va: ORDER BY co tien to "user_notes." nen
// Postgres phan giai duoc, va ghi chu dung SECTION duoc loc dung (khong lan sang ghi chu cua
// section khac trong cung khoa).
func TestListByCourse_LocTheoSection_KhongLoiAmbiguousColumn(t *testing.T) {
	db := openTestPostgres(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	defer sqlDB.Close()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("db.Begin(): %v", tx.Error)
	}
	// R5 test-data (danh dau ro de nhan biet, KHONG dung du lieu seed san) — ROLLBACK o cuoi,
	// khong ghi gi xuong DB that.
	defer tx.Rollback()

	suffix := uuid.NewString()
	instructor := model.User{
		Email:        "r5-test-instructor-" + suffix + "@40study.test",
		PasswordHash: "x",
		UserName:     "r5-test-instructor-" + suffix,
	}
	if err := tx.Create(&instructor).Error; err != nil {
		t.Fatalf("tao instructor: %v", err)
	}
	student := model.User{
		Email:        "r5-test-student-" + suffix + "@40study.test",
		PasswordHash: "x",
		UserName:     "r5-test-student-" + suffix,
	}
	if err := tx.Create(&student).Error; err != nil {
		t.Fatalf("tao student: %v", err)
	}
	course := model.Course{
		InstructorID: instructor.ID,
		Title:        "R5 test course " + suffix,
		Slug:         "r5-test-course-" + suffix,
		Price:        decimal.Zero,
	}
	if err := tx.Create(&course).Error; err != nil {
		t.Fatalf("tao course: %v", err)
	}
	sectionA := model.Section{CourseID: course.ID, Title: "Chuong A", DisplayOrder: 1}
	if err := tx.Create(&sectionA).Error; err != nil {
		t.Fatalf("tao sectionA: %v", err)
	}
	sectionB := model.Section{CourseID: course.ID, Title: "Chuong B", DisplayOrder: 2}
	if err := tx.Create(&sectionB).Error; err != nil {
		t.Fatalf("tao sectionB: %v", err)
	}
	lessonA := model.Lesson{SectionID: sectionA.ID, Title: "Bai A1", DisplayOrder: 1}
	if err := tx.Create(&lessonA).Error; err != nil {
		t.Fatalf("tao lessonA: %v", err)
	}
	lessonB := model.Lesson{SectionID: sectionB.ID, Title: "Bai B1", DisplayOrder: 1}
	if err := tx.Create(&lessonB).Error; err != nil {
		t.Fatalf("tao lessonB: %v", err)
	}
	noteA := model.UserNote{UserID: student.ID, LessonID: lessonA.ID, CourseID: &course.ID, Content: "ghi chu chuong A"}
	if err := tx.Create(&noteA).Error; err != nil {
		t.Fatalf("tao noteA: %v", err)
	}
	noteB := model.UserNote{UserID: student.ID, LessonID: lessonB.ID, CourseID: &course.ID, Content: "ghi chu chuong B"}
	if err := tx.Create(&noteB).Error; err != nil {
		t.Fatalf("tao noteB: %v", err)
	}

	repo := NewNoteRepository(tx)
	notes, err := repo.ListByCourse(context.Background(), student.ID, course.ID, &sectionA.ID, "newest")
	if err != nil {
		t.Fatalf("ListByCourse voi sectionID != nil tra loi = %v, muon khong loi (day chinh la R2/loi ambiguous column cua bao cao)", err)
	}
	if len(notes) != 1 {
		t.Fatalf("ListByCourse tra ve %d ghi chu, muon 1 (chi ghi chu cua sectionA)", len(notes))
	}
	if notes[0].ID != noteA.ID {
		t.Fatalf("ListByCourse tra ve ghi chu id=%s, muon noteA id=%s (loc sai section)", notes[0].ID, noteA.ID)
	}
}
