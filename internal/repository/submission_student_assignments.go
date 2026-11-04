package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
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
	NotStarted int64
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
	submitted := r.db.Table("submissions").
		Select("assignment_id, BOOL_OR(verdict = ?) AS accepted", model.VerdictAccepted).
		Where("user_id = ? AND deleted_at IS NULL", studentID).
		Group("assignment_id")

	var out StudentAssignmentStats
	err := r.studentClassAssignments(ctx, studentID).
		Joins("LEFT JOIN (?) s ON s.assignment_id = assignments.id", submitted).
		Select(`COUNT(*) AS total,
			COUNT(*) FILTER (WHERE s.accepted) AS completed,
			COUNT(*) FILTER (WHERE s.assignment_id IS NOT NULL AND NOT s.accepted) AS in_progress,
			COUNT(*) FILTER (WHERE s.assignment_id IS NULL AND (assignments.end_time IS NULL OR assignments.end_time >= ?)) AS not_started,
			COUNT(*) FILTER (WHERE s.assignment_id IS NULL AND assignments.end_time < ?) AS overdue`, now, now).
		Scan(&out).Error
	return out, err
}
