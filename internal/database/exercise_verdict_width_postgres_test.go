package database

// exercise_submissions.verdict varchar(20) -> varchar(32) (contract sec. 2 và 6). AutoMigrate của GORM KHÔNG nới
// cột này trên DB cũ: driver postgres báo kiểu "character varying", alias "varchar" làm isSameType=true nên bước so
// độ dài bị bỏ qua (gorm migrator.go MigrateColumn). Không có widenExerciseSubmissionVerdict thì
// memory_limit_exceeded (21 ký tự) vẫn không ghi được trên DB đang chạy và test này ĐỎ.

import (
	"testing"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func exerciseVerdictWidth(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var n int
	if err := db.Raw(`SELECT COALESCE(character_maximum_length, 0) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'exercise_submissions' AND column_name = 'verdict'`).
		Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestExerciseVerdictWidth_PG_WidensLegacyColumnKeepingRows(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(db *gorm.DB) error {
		return db.Exec(`CREATE TABLE exercise_submissions (id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			verdict varchar(20) DEFAULT 'pending');
			INSERT INTO exercise_submissions (verdict) VALUES ('accepted');`).Error
	})
	if got := exerciseVerdictWidth(t, db); got != 20 {
		t.Fatalf("schema giả lập DB cũ phải có varchar(20), thấy %d", got)
	}
	for i := 0; i < 2; i++ { // chạy lại khi khởi động lần sau: không lỗi, không đổi
		if err := widenExerciseSubmissionVerdict(db); err != nil {
			t.Fatalf("lần %d: %v", i+1, err)
		}
		if got := exerciseVerdictWidth(t, db); got != 32 {
			t.Fatalf("lần %d: verdict varchar(%d), muốn 32", i+1, got)
		}
	}
	if err := db.Exec(`INSERT INTO exercise_submissions (verdict) VALUES (?)`, string(model.VerdictMemoryLimitExceed)).Error; err != nil {
		t.Fatalf("memory_limit_exceeded vẫn không ghi được: %v", err)
	}
	var kept int64
	db.Raw(`SELECT count(*) FROM exercise_submissions WHERE verdict = 'accepted'`).Scan(&kept)
	if kept != 1 {
		t.Errorf("dòng cũ mất sau khi nới cột: %d", kept)
	}
}

func TestExerciseVerdictWidth_PG_NoTableIsNoop(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := widenExerciseSubmissionVerdict(db); err != nil {
		t.Fatalf("DB chưa có bảng phải bỏ qua: %v", err)
	}
}
