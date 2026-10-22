package database

// exercise_submission_pending_cleanup.go — dọn bài nộp bài tập kẹt 'pending' do đường nộp cũ để lại.
//
// POST /exercises/:id/submit (đã gỡ) tạo một dòng exercise_submissions verdict='pending' rồi đẩy tin
// vào hàng RabbitMQ exercise.submit — hàng này KHÔNG có consumer nào, nên dòng đó không bao giờ được
// chấm và hiện mãi là "bài nộp gần nhất" (PENDING) của bài học. Đường chấm hiện tại
// (POST /lessons/:lessonId/exercises/:id/submit) chấm đồng bộ và không bao giờ lưu 'pending', nên mọi
// dòng 'pending' còn lại đều là rác của đường cũ.
//
// Đánh dấu MỀM thành 'system_error' kèm lý do (không xoá: giữ mã nguồn học viên đã nộp). Repository
// GetLatestSubmission bỏ qua cả 'pending' lẫn 'system_error', nên các dòng này không hiện là bài nộp
// gần nhất nữa. Chạy đúng một lần mỗi DB (runDataMigrationOnce); câu UPDATE tự nó idempotent.
// Khôi phục: UPDATE exercise_submissions SET verdict='pending', error_message=NULL
//            WHERE id IN (<id đã log>);

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
)

// exerciseSubmissionPendingCleanupName: tên bản ghi đánh dấu trong data_migrations.
const exerciseSubmissionPendingCleanupName = "exercise_submission_pending_cleanup"

// exerciseSubmissionPendingCleanupReason: error_message ghi vào dòng đã dọn.
const exerciseSubmissionPendingCleanupReason = "legacy queued submission was never judged (POST /exercises/:id/submit removed); submit again"

// exerciseSubmissionPendingLogCap giới hạn số id ghi vào log mỗi lần chạy.
const exerciseSubmissionPendingLogCap = 200

const exerciseSubmissionPendingCleanupSQL = `
	UPDATE exercise_submissions
	SET verdict = 'system_error', error_message = ?, updated_at = now()
	WHERE verdict = 'pending'
	RETURNING id;
`

// runExerciseSubmissionPendingCleanup chạy trong RunPostMigrations, đúng một lần trên mỗi DB.
func runExerciseSubmissionPendingCleanup(db *gorm.DB) error {
	if err := runDataMigrationOnce(db, exerciseSubmissionPendingCleanupName, markPendingExerciseSubmissions); err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "mark legacy pending exercise_submissions system_error", err)
	}
	return nil
}

// markPendingExerciseSubmissions đánh dấu mọi dòng 'pending' và log id đã đổi.
func markPendingExerciseSubmissions(tx *gorm.DB) error {
	var ids []string
	if err := tx.Raw(exerciseSubmissionPendingCleanupSQL, exerciseSubmissionPendingCleanupReason).Scan(&ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	logged, suffix := ids, ""
	if len(ids) > exerciseSubmissionPendingLogCap {
		logged, suffix = ids[:exerciseSubmissionPendingLogCap], fmt.Sprintf(" (first %d ids)", exerciseSubmissionPendingLogCap)
	}
	log.Printf("[migrate] exercise_submissions: marked %d legacy pending submission(s) system_error%s: %s", len(ids), suffix, strings.Join(logged, ","))
	return nil
}
