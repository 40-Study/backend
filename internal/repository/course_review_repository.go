package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// Phase 3 duyệt khoá học (2026-09-28) — contract:
// plans/260927-2055-role-based-ux-qa/admin-features/phase-03-course-teacher-approval.md
var (
	ErrCourseReviewNotFound      = errors.New("course not found")
	ErrCourseReviewNotOwner      = errors.New("forbidden: not the owner")
	ErrCourseInvalidReviewStatus = errors.New("invalid course status for this review action")
)

// CourseReviewAction — 3 hành động của luồng duyệt.
type CourseReviewAction string

const (
	CourseActionSubmit  CourseReviewAction = "submit"
	CourseActionApprove CourseReviewAction = "approve"
	CourseActionReject  CourseReviewAction = "reject"
)

// EvaluateCourseReviewTransition là hàm THUẦN (không DB) quyết định trạng thái mới của khoá học
// cho 1 hành động duyệt — tách riêng để unit test mọi cặp (trạng thái hiện tại, hành động):
//   - submit : draft|rejected -> pending_review (giáo viên nộp / nộp lại)
//   - approve: pending_review -> published   (chỉ admin)
//   - reject : pending_review -> rejected    (chỉ admin)
//
// Mọi cặp khác trả ErrCourseInvalidReviewStatus — đặc biệt KHÔNG có đường nào tới published ngoài
// approve, nên giáo viên không tự xuất bản được.
func EvaluateCourseReviewTransition(current string, action CourseReviewAction) (string, error) {
	switch action {
	case CourseActionSubmit:
		if current == model.CourseStatusDraft || current == model.CourseStatusRejected {
			return model.CourseStatusPendingReview, nil
		}
	case CourseActionApprove:
		if current == model.CourseStatusPendingReview {
			return model.CourseStatusPublished, nil
		}
	case CourseActionReject:
		if current == model.CourseStatusPendingReview {
			return model.CourseStatusRejected, nil
		}
	}
	return "", ErrCourseInvalidReviewStatus
}

// AdminCourseReviewFilter — tham số hàng chờ duyệt khoá học của admin.
type AdminCourseReviewFilter struct {
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

type CourseReviewRepositoryInterface interface {
	// ApplyReviewAction khoá dòng course (FOR UPDATE), kiểm transition bằng
	// EvaluateCourseReviewTransition rồi ghi — 2 admin bấm duyệt/từ chối cùng lúc sẽ xếp hàng,
	// người thứ 2 thấy trạng thái đã đổi và nhận ErrCourseInvalidReviewStatus.
	// ownerID != nil: bắt buộc course.instructor_id == *ownerID (luồng giáo viên nộp duyệt).
	ApplyReviewAction(ctx context.Context, courseID uuid.UUID, action CourseReviewAction,
		ownerID *uuid.UUID, reviewerID *uuid.UUID, reason *string) (*model.Course, error)
	ListForReview(ctx context.Context, filter AdminCourseReviewFilter) ([]model.Course, int64, error)
}

type CourseReviewRepository struct {
	db *gorm.DB
}

func NewCourseReviewRepository(db *gorm.DB) *CourseReviewRepository {
	return &CourseReviewRepository{db: db}
}

func (r *CourseReviewRepository) ApplyReviewAction(
	ctx context.Context,
	courseID uuid.UUID,
	action CourseReviewAction,
	ownerID *uuid.UUID,
	reviewerID *uuid.UUID,
	reason *string,
) (*model.Course, error) {
	var result model.Course
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var course model.Course
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", courseID).First(&course).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCourseReviewNotFound
			}
			return err
		}
		if ownerID != nil && course.InstructorID != *ownerID {
			return ErrCourseReviewNotOwner
		}
		next, err := EvaluateCourseReviewTransition(course.Status, action)
		if err != nil {
			return err
		}

		now := time.Now()
		updates := map[string]interface{}{"status": next}
		switch action {
		case CourseActionSubmit:
			updates["submitted_at"] = now
		case CourseActionApprove:
			updates["published_at"] = now
			updates["reviewed_by"] = reviewerID
			updates["reviewed_at"] = now
			updates["rejection_reason"] = nil
		case CourseActionReject:
			updates["reviewed_by"] = reviewerID
			updates["reviewed_at"] = now
			updates["rejection_reason"] = reason
		}
		if err := tx.Model(&model.Course{}).Where("id = ?", course.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", course.ID).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *CourseReviewRepository) ListForReview(ctx context.Context, filter AdminCourseReviewFilter) ([]model.Course, int64, error) {
	var courses []model.Course
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Course{}).
		Joins("JOIN users ON users.id = courses.instructor_id").
		Where("courses.status = ?", filter.Status)
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		query = utils.ApplyKeywordSearch(query, kw, "courses.title", "users.email", "users.full_name")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Hàng chờ: khoá nộp gần nhất lên đầu; khoá chưa từng nộp (NULL) xuống cuối.
	if err := utils.ApplyPagination(query, filter.Page, filter.PageSize).
		Select("courses.*").
		Preload("Instructor").
		Order("courses.submitted_at DESC NULLS LAST").
		Order("courses.created_at DESC").
		Find(&courses).Error; err != nil {
		return nil, 0, err
	}
	return courses, total, nil
}
