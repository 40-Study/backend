package database

import (
	"fmt"

	"gorm.io/gorm"
)

// exerciseVerdictColumnWidth: độ dài cột exercise_submissions.verdict theo contract sec. 2 (verdict dài nhất hiện có,
// memory_limit_exceeded, là 21 ký tự; varchar(20) cũ không chứa được).
const exerciseVerdictColumnWidth = 32

// widenExerciseSubmissionVerdict nới exercise_submissions.verdict lên varchar(32) trên DB cũ. Cần tường minh vì
// AutoMigrate không tự nới: driver postgres báo kiểu "character varying", alias "varchar" làm GORM coi là cùng kiểu
// và bỏ qua bước so độ dài. Idempotent: chỉ ALTER khi cột đang ngắn hơn; bảng/cột chưa có thì không làm gì. Nới
// varchar trong Postgres chỉ đổi metadata, không ghi lại bảng.
func widenExerciseSubmissionVerdict(db *gorm.DB) error {
	sql := fmt.Sprintf(`
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.columns
			           WHERE table_schema = current_schema() AND table_name = 'exercise_submissions'
			             AND column_name = 'verdict' AND character_maximum_length < %[1]d) THEN
				ALTER TABLE exercise_submissions ALTER COLUMN verdict TYPE varchar(%[1]d);
			END IF;
		END $$;
	`, exerciseVerdictColumnWidth)
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("nới exercise_submissions.verdict lên varchar(%d): %w", exerciseVerdictColumnWidth, err)
	}
	return nil
}
