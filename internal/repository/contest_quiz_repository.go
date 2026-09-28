package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ContestEligibleQuestionTypes — loại câu chấm tự động được (contract §3.3, không có essay).
var ContestEligibleQuestionTypes = []string{"single_choice", "multiple_choice", "true_false", "fill_blank"}

// ContestQuizInfo — dữ kiện để service quyết định quiz có dùng được cho cuộc thi không.
// Đọc created_by bằng SQL thô (không qua model.Quiz) để không phụ thuộc thứ tự merge với lane B2.
type ContestQuizInfo struct {
	ID              uuid.UUID
	Title           string
	LessonID        *uuid.UUID
	CourseID        *uuid.UUID
	SessionID       *uuid.UUID
	CreatedBy       *uuid.UUID
	QuestionCount   int
	TotalPoints     decimal.Decimal
	IneligibleCount int
	AttemptCount    int
	ContestID       *uuid.UUID // cuộc thi đang gắn (mọi status)
}

const contestQuizInfoSelect = `
	SELECT q.id, q.title, q.lesson_id, q.course_id, q.session_id, q.created_by,
	       (SELECT COUNT(*) FROM questions x WHERE x.quiz_id = q.id AND x.deleted_at IS NULL) AS question_count,
	       (SELECT COALESCE(SUM(x.points), 0) FROM questions x WHERE x.quiz_id = q.id AND x.deleted_at IS NULL) AS total_points,
	       (SELECT COUNT(*) FROM questions x WHERE x.quiz_id = q.id AND x.deleted_at IS NULL
	           AND x.question_type NOT IN ?) AS ineligible_count,
	       (SELECT COUNT(*) FROM quiz_attempts a WHERE a.quiz_id = q.id) AS attempt_count,
	       (SELECT c.id FROM contests c WHERE c.quiz_id = q.id LIMIT 1) AS contest_id
	FROM quizzes q WHERE q.deleted_at IS NULL`

func (r *ContestRepository) GetQuizInfo(ctx context.Context, quizID uuid.UUID) (*ContestQuizInfo, error) {
	var rows []ContestQuizInfo
	err := r.db.WithContext(ctx).Raw(contestQuizInfoSelect+` AND q.id = ?`, ContestEligibleQuestionTypes, quizID).
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// ListEligibleQuizzes — #4 quiz-options: standalone, đủ điều kiện, chưa gắn cuộc thi nào.
// ownerID nil = admin (mọi quiz standalone).
func (r *ContestRepository) ListEligibleQuizzes(ctx context.Context, ownerID *uuid.UUID) ([]ContestQuizInfo, error) {
	sql := `SELECT * FROM (` + contestQuizInfoSelect + `
		AND q.lesson_id IS NULL AND q.course_id IS NULL AND q.session_id IS NULL`
	args := []interface{}{ContestEligibleQuestionTypes}
	if ownerID != nil {
		sql += ` AND q.created_by = ?`
		args = append(args, *ownerID)
	}
	sql += `) x WHERE question_count > 0 AND ineligible_count = 0 AND attempt_count = 0 AND contest_id IS NULL
		ORDER BY title, id`
	var rows []ContestQuizInfo
	err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error
	return rows, err
}

func (r *ContestRepository) CourseExists(ctx context.Context, courseID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("courses").Where("id = ? AND deleted_at IS NULL", courseID).Count(&n).Error
	return n > 0, err
}

// UnusableVoucherIDs trả những voucher KHÔNG phát được: không tồn tại, đã xoá, tắt, hoặc hết hạn.
// Cùng điều kiện với VoucherService.GrantVoucherTx (lane B2) để lỗi hiện sớm lúc duyệt/sửa giải.
func (r *ContestRepository) UnusableVoucherIDs(ctx context.Context, ids []uuid.UUID, now time.Time) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var usable []uuid.UUID
	err := r.db.WithContext(ctx).Raw(`
		SELECT id FROM vouchers
		WHERE id IN ? AND deleted_at IS NULL AND is_active = true AND (end_date IS NULL OR end_date > ?)`,
		ids, now).Scan(&usable).Error
	if err != nil {
		return nil, err
	}
	ok := map[uuid.UUID]bool{}
	for _, id := range usable {
		ok[id] = true
	}
	var bad []uuid.UUID
	for _, id := range ids {
		if !ok[id] {
			bad = append(bad, id)
		}
	}
	return bad, nil
}
