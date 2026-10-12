package database

// enrollment_total_lessons_backfill.go — QA 261010 follow-up 2c/2d, quan sát 1: ghi danh từng được tạo với
// total_lessons = 0 và chỉ có tổng thật sau lần tính lại tiến độ đầu tiên (hiện "0/0 bài"). Code mới đặt tổng ngay lúc
// ghi danh (EnrollmentRepository.Create / RestoreAndReactivate); bước này sửa các dòng CŨ còn kẹt ở 0 dù khoá đã có
// bài bắt buộc, kể cả dòng đã xoá mềm (sẽ được khôi phục khi ghi danh lại, nhưng đọc trực tiếp vẫn nên đúng).
//
// Đếm đúng như EnrollmentRepository.CountTotalMandatory (bài bắt buộc của chương chưa xoá mềm). Chỉ chạm dòng
// total_lessons = 0 và chỉ khi khoá có ít nhất một bài bắt buộc, nên: khoá thật sự chưa có bài giữ nguyên 0, và chạy
// lại là no-op. Không đụng completed_lessons / progress_percentage (0 tổng thì hai cột đó vẫn đúng ở 0).
//
// CHẠY ĐÚNG MỘT LẦN trên mỗi DB (runDataMigrationOnce, dấu trong data_migrations). Từ đây đường ghi danh luôn đặt tổng,
// nên không cần chạy mỗi lần khởi động.

import (
	"fmt"

	"gorm.io/gorm"
)

const enrollmentTotalLessonsBackfillName = "enrollment_total_lessons_backfill"

const enrollmentTotalLessonsBackfillSQL = `
	UPDATE enrollments e
	SET total_lessons = t.total
	FROM (
		SELECT s.course_id, COUNT(*) AS total
		FROM lessons l
		JOIN sections s ON s.id = l.section_id
		WHERE s.deleted_at IS NULL AND l.is_mandatory = true
		GROUP BY s.course_id
	) t
	WHERE t.course_id = e.course_id AND e.total_lessons = 0
`

// backfillEnrollmentTotalLessons chạy câu SQL trên tx và trả số dòng đã sửa (test dùng để kiểm "lần hai sửa 0").
func backfillEnrollmentTotalLessons(tx *gorm.DB) (int64, error) {
	res := tx.Exec(enrollmentTotalLessonsBackfillSQL)
	return res.RowsAffected, res.Error
}

// runEnrollmentTotalLessonsBackfill — xem chú thích đầu file. Gọi từ RunPostMigrations sau AutoMigrate.
func runEnrollmentTotalLessonsBackfill(db *gorm.DB) error {
	err := runDataMigrationOnce(db, enrollmentTotalLessonsBackfillName, func(tx *gorm.DB) error {
		_, err := backfillEnrollmentTotalLessons(tx)
		return err
	})
	if err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "backfill enrollments.total_lessons", err)
	}
	return nil
}
