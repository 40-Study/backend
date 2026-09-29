package database

// quiz_created_by_backfill.go — gán chủ cho quiz tạo trước khi có cột quizzes.created_by (lane B2
// "Cuộc thi", chủ dự án chốt 28/09). Từ khi QuizService chỉ cho người tạo hoặc admin sửa quiz,
// quiz có created_by NULL chỉ admin sửa được; không backfill thì giảng viên mất quyền sửa quiz
// trong bài học của chính họ.
//
// Idempotent: mọi câu chỉ chạm dòng created_by IS NULL, nên chạy lại ở mỗi lần API khởi động không
// đổi gì và không bao giờ ghi đè chủ đã có. Quiz không suy được chủ giữ NULL (chỉ admin sửa).

import (
	"fmt"

	"gorm.io/gorm"
)

// quizCreatedByBackfillStatements theo thứ tự ưu tiên: một quiz chỉ gắn một trong lesson/course/
// session (xem model.Quiz); quiz cuộc thi luôn standalone nên không chồng lên ba loại đầu.
func quizCreatedByBackfillStatements() []struct{ name, sql string } {
	return []struct{ name, sql string }{
		{
			name: "backfill quizzes.created_by from lesson -> section -> course instructor",
			sql: `
				UPDATE quizzes q
				SET created_by = c.instructor_id
				FROM lessons l
				JOIN sections s ON s.id = l.section_id
				JOIN courses c ON c.id = s.course_id
				WHERE q.created_by IS NULL AND q.lesson_id = l.id;
			`,
		},
		{
			name: "backfill quizzes.created_by from course instructor",
			sql: `
				UPDATE quizzes q
				SET created_by = c.instructor_id
				FROM courses c
				WHERE q.created_by IS NULL AND q.lesson_id IS NULL AND q.course_id = c.id;
			`,
		},
		{
			// Quiz live: người tạo là host của buổi live (giảng viên chạy buổi đó).
			name: "backfill quizzes.created_by from livestream session host",
			sql: `
				UPDATE quizzes q
				SET created_by = ls.host_id
				FROM livestream_sessions ls
				WHERE q.created_by IS NULL AND q.lesson_id IS NULL AND q.course_id IS NULL
				  AND q.session_id = ls.id;
			`,
		},
		{
			// contests.quiz_id do lane B1 thêm. Trước khi B1 merge cột chưa có, nên kiểm
			// information_schema và dùng EXECUTE để câu lệnh không bị phân tích khi thiếu cột.
			name: "backfill quizzes.created_by from contest creator",
			sql: `
				DO $$
				BEGIN
					IF EXISTS (SELECT 1 FROM information_schema.columns
					           WHERE table_schema = current_schema() AND table_name = 'contests'
					             AND column_name = 'quiz_id') THEN
						EXECUTE 'UPDATE quizzes q SET created_by = ct.created_by
						         FROM contests ct
						         WHERE q.created_by IS NULL AND ct.quiz_id = q.id';
					END IF;
				END $$;
			`,
		},
	}
}

// runQuizCreatedByBackfill chạy các câu backfill; gọi từ RunPostMigrations sau các post-migration
// khác (AutoMigrate đã thêm cột created_by và contests.quiz_id trước đó).
func runQuizCreatedByBackfill(db *gorm.DB) error {
	for _, stmt := range quizCreatedByBackfillStatements() {
		if err := db.Exec(stmt.sql).Error; err != nil {
			return fmt.Errorf("post-migration %q failed: %w", stmt.name, err)
		}
	}
	return nil
}
