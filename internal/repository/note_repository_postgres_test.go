package repository

// R5 (code-reviewer-260919-1557): "column reference \"created_at\" is ambiguous" CHI xuat hien
// khi Postgres THAT SU phan giai cau lenh (ListByCourse JOIN "lessons" khi co sectionID, va
// bang do CUNG co cot created_at) — mot repo gia lap (mock/fake) khong bao gio bat duoc loi
// nay, vi lo hong nam trong CHINH cau SQL. Test nay ket noi Postgres THAT (khong mock), tao du
// lieu rieng trong MOT transaction roi ROLLBACK, va bo qua (t.Skip) neu khong ket noi duoc — day
// la test tich hop bo sung, khong thay the cac test dry-run/mock o noi khac.

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"study.com/v1/internal/model"
)

// loadEnvFileForTest doc file .env o repo root theo duong dan TUONG DOI VOI FILE NAY (khong phu
// thuoc cwd cua `go test`), roi set vao os.Setenv cho NHUNG bien CHUA duoc set san — bien moi
// truong that su (CI, shell da export) luon thang gia tri trong file. Khong dung them thu vien
// nao (godotenv) chi de phuc vu MOT test.
func loadEnvFileForTest(t *testing.T) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	envPath := filepath.Join(repoRoot, ".env")
	f, err := os.Open(envPath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// openTestPostgres mo mot ket noi Postgres THAT toi DB dev cuc bo (127.0.0.1:5432/study_db —
// xem CLAUDE.md moi truong worktree). Bo qua test (khong FAIL) neu khong ket noi duoc, de suite
// khong do gay tren mot may khong co Postgres chay san.
func openTestPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	loadEnvFileForTest(t)

	host := envOrDefault("DB_HOST", "localhost")
	port := envOrDefault("DB_PORT", "5432")
	user := envOrDefault("DB_USER", "study_user")
	pass := os.Getenv("DB_PASSWORD")
	name := envOrDefault("DB_NAME", "study_db")

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Ho_Chi_Minh",
		host, user, pass, name, port)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("khong ket noi duoc Postgres that (%s:%s/%s) — bo qua test tich hop R5: %v", host, port, name, err)
	}
	return db
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
