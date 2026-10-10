package database

// lesson_content_backfill.go — QA T2 / plan 261008 D2: quiz gắn bài (quizzes.lesson_id) từng không có dòng nào
// trong lesson_contents, nên danh sách nội dung của bài không hiện chúng. Bước này cho MỌI quiz gắn bài chưa xoá
// mềm và chưa được dòng nội dung nào giữ (lesson_contents.quiz_id) một dòng type='quiz'. Không dùng điều kiện
// "bài chưa có nội dung nào": nhánh bài tập -> quiz tạo quiz gắn bài kể cả trên bài đã có video, và bài đó sẽ mất
// quiz khi trang học đọc theo nội dung.
//
// CHẠY ĐÚNG MỘT LẦN trên mỗi DB (runDataMigrationOnce, bản ghi đánh dấu trong data_migrations). Không chạy mỗi
// lần khởi động vì:
//   - giảng viên xoá dòng nội dung quiz (bỏ quiz khỏi bài, quiz vẫn còn) sẽ thấy nó "sống lại" sau lần deploy sau;
//   - giảng viên vừa tạo quiz gắn bài rồi mới gắn nội dung (POST contents với quiz_id) sẽ nhận 409
//     QUIZ_ALREADY_LINKED nếu API khởi động lại giữa hai bước.
// Câu SQL bản thân vẫn idempotent (NOT EXISTS), nên mất bản ghi đánh dấu cũng không sinh dòng trùng.
//
// Hand-off (phase 8): gọi runLessonContentQuizBackfill(db) trong RunPostMigrations SAU AutoMigrate (cần cột
// quiz_id) và SAU bước nới CHK chk_lesson_contents_type (cần cho phép type='quiz').

import (
	"fmt"

	"gorm.io/gorm"
)

const lessonContentQuizBackfillName = "lesson_content_quiz_backfill"

// display_order = (lớn nhất hiện có trong bài, -1 nếu bài trống) + số thứ tự của quiz trong bài (theo created_at,
// id) nên nhiều quiz mồ côi trên cùng một bài không trùng thứ tự. is_mandatory = false: các quiz này trước đây
// không là nội dung bắt buộc, backfill không được làm đổi hành vi học của học viên.
const lessonContentQuizBackfillSQL = `
	INSERT INTO lesson_contents (id, lesson_id, type, title, quiz_id, duration, is_mandatory, display_order, created_at, updated_at)
	SELECT gen_random_uuid(), o.lesson_id, 'quiz', o.title, o.id, 0, false,
	       COALESCE((SELECT MAX(lc.display_order) FROM lesson_contents lc WHERE lc.lesson_id = o.lesson_id), -1) + o.rn,
	       now(), now()
	FROM (
		SELECT q.id, q.lesson_id, q.title,
		       ROW_NUMBER() OVER (PARTITION BY q.lesson_id ORDER BY q.created_at, q.id) AS rn
		FROM quizzes q
		WHERE q.lesson_id IS NOT NULL
		  AND q.deleted_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM lesson_contents lc WHERE lc.quiz_id = q.id)
	) o
`

// backfillLessonContentQuizzes chạy câu SQL trên tx và trả số dòng đã chèn (test dùng để kiểm "lần hai chèn 0").
func backfillLessonContentQuizzes(tx *gorm.DB) (int64, error) {
	res := tx.Exec(lessonContentQuizBackfillSQL)
	return res.RowsAffected, res.Error
}

// runLessonContentQuizBackfill — xem chú thích đầu file. Gọi từ RunPostMigrations (phase 8).
func runLessonContentQuizBackfill(db *gorm.DB) error {
	err := runDataMigrationOnce(db, lessonContentQuizBackfillName, func(tx *gorm.DB) error {
		_, err := backfillLessonContentQuizzes(tx)
		return err
	})
	if err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "backfill quiz rows into lesson_contents", err)
	}
	return nil
}
