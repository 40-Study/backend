package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// Bài tập theo LỚP của một học sinh (cho màn "Bài tập" của phụ huynh). Danh sách được dựng từ bài tập của các lớp
// học sinh đang theo học, KHÔNG từ bài đã nộp: dựng từ bài nộp thì bài quá hạn mà chưa từng nộp biến mất khỏi mắt
// phụ huynh, đúng loại bài họ cần thấy nhất (QA3 P-01).

// StudentAssignmentStats đếm trên TOÀN BỘ bài tập của lớp (không chỉ trang hiện tại).
type StudentAssignmentStats struct {
	Total      int64
	Completed  int64
	InProgress int64
	// PendingReview/NeedsRetry (QA5 Q5-PG-13): InProgress tách đôi — bài nộp tốt nhất còn `pending` (đến lượt giảng
	// viên chấm) và bài nộp chưa đạt (con cần làm lại). PendingReview + NeedsRetry == InProgress.
	PendingReview int64
	NeedsRetry    int64
	NotStarted    int64
	Overdue    int64
}

// studentClassAssignments: bài tập đã xuất bản của mọi lớp (chưa xoá) mà học sinh đang là thành viên active.
// Không cắt theo ngày vào lớp: học sinh vào lớp sau hạn nộp vẫn thấy bài đó là quá hạn.
func (r *SubmissionRepository) studentClassAssignments(ctx context.Context, studentID uuid.UUID) *gorm.DB {
	activeClassIDs := r.db.Table("student_classes sc").
		Select("sc.class_id").
		Joins("JOIN classes c ON c.id = sc.class_id AND c.deleted_at IS NULL").
		Where("sc.student_id = ? AND sc.status = ?", studentID, "active")
	return r.db.WithContext(ctx).Model(&model.Assignment{}).
		Where("assignments.is_published = ?", true).
		Where("assignments.class_id IN (?)", activeClassIDs)
}

// ListStudentClassAssignments trả một trang bài tập (hạn nộp mới nhất trước, bài không hạn cuối) kèm lớp.
func (r *SubmissionRepository) ListStudentClassAssignments(ctx context.Context, studentID uuid.UUID, page, pageSize int) ([]model.Assignment, int64, error) {
	query := r.studentClassAssignments(ctx, studentID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var assignments []model.Assignment
	err := utils.ApplyPagination(query, page, pageSize).
		Preload("Class").
		Order("assignments.end_time DESC NULLS LAST, assignments.created_at DESC, assignments.id").
		Find(&assignments).Error
	return assignments, total, err
}

// ListUserSubmissionsForAssignments trả mọi bài nộp của userID cho các bài tập đã cho (mới nhất trước).
func (r *SubmissionRepository) ListUserSubmissionsForAssignments(ctx context.Context, userID uuid.UUID, assignmentIDs []uuid.UUID) ([]model.Submission, error) {
	if len(assignmentIDs) == 0 {
		return nil, nil
	}
	var submissions []model.Submission
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND assignment_id IN ?", userID, assignmentIDs).
		Order("created_at DESC").
		Find(&submissions).Error
	return submissions, err
}

// StudentClassAssignmentStats đếm theo trạng thái: completed = có bài nộp accepted; in_progress = đã nộp nhưng chưa
// accepted; chưa nộp thì overdue khi quá hạn (end_time < now), còn lại not_started. Cùng quy tắc với
// ParentDashboardService.childAssignmentStatus.
func (r *SubmissionRepository) StudentClassAssignmentStats(ctx context.Context, studentID uuid.UUID, now time.Time) (StudentAssignmentStats, error) {
	// best_verdict: verdict của bài nộp "tốt nhất" (nhiều test qua nhất, hoà thì mới nhất) — cùng quy tắc
	// buildChildAssignmentDto, để ô thống kê khớp trạng thái từng dòng.
	submitted := r.db.Table("submissions s0").
		Select(`s0.assignment_id,
			BOOL_OR(s0.verdict = ?) AS accepted,
			(SELECT b.verdict FROM submissions b WHERE b.assignment_id = s0.assignment_id AND b.user_id = s0.user_id AND b.deleted_at IS NULL
			 ORDER BY b.test_cases_passed DESC, b.created_at DESC LIMIT 1) AS best_verdict`, model.VerdictAccepted).
		Where("s0.user_id = ? AND s0.deleted_at IS NULL", studentID).
		Group("s0.assignment_id, s0.user_id")

	var out StudentAssignmentStats
	err := r.studentClassAssignments(ctx, studentID).
		Joins("LEFT JOIN (?) s ON s.assignment_id = assignments.id", submitted).
		Select(`COUNT(*) AS total,
			COUNT(*) FILTER (WHERE s.accepted) AS completed,
			COUNT(*) FILTER (WHERE s.assignment_id IS NOT NULL AND NOT s.accepted) AS in_progress,
			COUNT(*) FILTER (WHERE s.assignment_id IS NOT NULL AND NOT s.accepted AND (s.best_verdict IS NULL OR s.best_verdict = 'pending')) AS pending_review,
			COUNT(*) FILTER (WHERE s.assignment_id IS NOT NULL AND NOT s.accepted AND s.best_verdict IS NOT NULL AND s.best_verdict <> 'pending') AS needs_retry,
			COUNT(*) FILTER (WHERE s.assignment_id IS NULL AND (assignments.end_time IS NULL OR assignments.end_time >= ?)) AS not_started,
			COUNT(*) FILTER (WHERE s.assignment_id IS NULL AND assignments.end_time < ?) AS overdue`, now, now).
		Scan(&out).Error
	return out, err
}

// MySubmissionSummary: bài nộp MỚI NHẤT của người gọi cho một bài tập, kèm điểm của lần chấm mới nhất (nếu đã chấm).
// Score/MaxScore nil = chưa có điểm (contract C1).
type MySubmissionSummary struct {
	AssignmentID uuid.UUID
	ID           uuid.UUID
	SubmittedAt  time.Time
	Score        *decimal.Decimal
	MaxScore     *decimal.Decimal
}

// ListMySubmissionSummaries trả, theo assignment_id, bài nộp mới nhất của userID trong số assignmentIDs (bài nào
// chưa nộp thì không có khoá). Một truy vấn cho cả trang, không N+1.
func (r *SubmissionRepository) ListMySubmissionSummaries(ctx context.Context, userID uuid.UUID, assignmentIDs []uuid.UUID) (map[uuid.UUID]MySubmissionSummary, error) {
	out := make(map[uuid.UUID]MySubmissionSummary, len(assignmentIDs))
	if len(assignmentIDs) == 0 {
		return out, nil
	}
	var rows []MySubmissionSummary
	err := r.db.WithContext(ctx).Raw(`SELECT DISTINCT ON (s.assignment_id)
			s.assignment_id AS assignment_id, s.id AS id, s.created_at AS submitted_at, g.score AS score, g.max_score AS max_score
		FROM submissions s
		LEFT JOIN LATERAL (
			SELECT score, max_score FROM grades
			WHERE student_id = s.user_id AND assignment_id = s.assignment_id AND deleted_at IS NULL
			ORDER BY graded_at DESC, created_at DESC LIMIT 1
		) g ON TRUE
		WHERE s.user_id = ? AND s.assignment_id IN ? AND s.deleted_at IS NULL
		ORDER BY s.assignment_id, s.created_at DESC`, userID, assignmentIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.AssignmentID] = row
	}
	return out, nil
}
