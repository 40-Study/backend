package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// ExerciseSubmissionResultRepositoryInterface: kết quả từng test case của bài nộp bài tập khoá học (contract sec. 6
// và 7). Người ghi duy nhất là grader (BE-B); người đọc là chi tiết bài nộp (BE-C, E6).
type ExerciseSubmissionResultRepositoryInterface interface {
	// SaveGradingOutcome ghi verdict tổng của sub và thay TOÀN BỘ kết quả cũ của nó bằng rows, trong một
	// transaction. Gọi lại với cùng bài nộp (RabbitMQ redelivery) không nhân đôi dòng. Bài nộp không còn tồn tại
	// -> lỗi bọc gorm.ErrRecordNotFound và không ghi gì (grader ACK và bỏ qua thay vì requeue mãi).
	SaveGradingOutcome(ctx context.Context, sub *model.ExerciseSubmission, rows []model.ExerciseSubmissionResult) error
	// ListBySubmission trả kết quả theo display_order tăng dần.
	ListBySubmission(ctx context.Context, submissionID uuid.UUID) ([]model.ExerciseSubmissionResult, error)
}

type ExerciseSubmissionResultRepository struct {
	db *gorm.DB
}

func NewExerciseSubmissionResultRepository(db *gorm.DB) *ExerciseSubmissionResultRepository {
	return &ExerciseSubmissionResultRepository{db: db}
}

// gradedSubmissionColumns: cột bài nộp mà grader được ghi (không đụng code/language/user).
var gradedSubmissionColumns = []string{"Verdict", "TestCasesPassed", "TotalTestCases", "ExecutionTimeMs", "MemoryUsedKB", "ErrorMessage"}

func (r *ExerciseSubmissionResultRepository) SaveGradingOutcome(ctx context.Context, sub *model.ExerciseSubmission, rows []model.ExerciseSubmissionResult) error {
	if sub == nil || sub.ID == uuid.Nil {
		return errors.New("save grading outcome: submission id is required")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Cập nhật bài nộp trước: không còn dòng nào thì dừng trước khi chèn kết quả (FK sẽ chặn chèn).
		res := tx.Model(sub).Select(gradedSubmissionColumns).Updates(sub)
		if res.Error != nil {
			return fmt.Errorf("save grading outcome: update submission %s: %w", sub.ID, res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("save grading outcome: submission %s: %w", sub.ID, gorm.ErrRecordNotFound)
		}
		if err := tx.Where("submission_id = ?", sub.ID).Delete(&model.ExerciseSubmissionResult{}).Error; err != nil {
			return fmt.Errorf("save grading outcome: delete earlier results: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		fresh := make([]model.ExerciseSubmissionResult, len(rows))
		for i, row := range rows {
			row.ID = uuid.New() // dòng mới mỗi lần chấm; không tin ID người gọi truyền vào
			row.SubmissionID = sub.ID
			row.Submission = nil
			fresh[i] = row
		}
		if err := tx.Create(&fresh).Error; err != nil {
			return fmt.Errorf("save grading outcome: insert results: %w", err)
		}
		return nil
	})
}

func (r *ExerciseSubmissionResultRepository) ListBySubmission(ctx context.Context, submissionID uuid.UUID) ([]model.ExerciseSubmissionResult, error) {
	var rows []model.ExerciseSubmissionResult
	err := r.db.WithContext(ctx).
		Where("submission_id = ?", submissionID).
		Order("display_order ASC, created_at ASC").
		Find(&rows).Error
	return rows, err
}
