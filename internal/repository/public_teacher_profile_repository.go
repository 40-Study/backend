package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// maxPublicTeacherCourses chặn số khoá trả về ở trang giảng viên công khai (thống kê vẫn tính trên MỌI khoá published).
const maxPublicTeacherCourses = 100

// PublicTeacherCourseStats: tổng hợp trên các khoá published của một giảng viên. RatingPoints = Σ(average_rating × total_reviews),
// để rating trung bình có trọng số theo số đánh giá được tính ở tầng service (không lưu cột dẫn xuất).
type PublicTeacherCourseStats struct {
	CourseCount  int64
	StudentCount int64
	ReviewCount  int64
	RatingPoints decimal.Decimal
}

type PublicTeacherProfileRepositoryInterface interface {
	// GetTeacher: user đang hoạt động, chưa xoá, đang giữ vai trò TEACHER còn hiệu lực; nil khi không phải.
	GetTeacher(ctx context.Context, id uuid.UUID) (*model.User, error)
	// GetApprovedProfile: hồ sơ giảng viên đã duyệt; nil khi chưa có hoặc chưa/không được duyệt.
	GetApprovedProfile(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error)
	PublishedCourseStats(ctx context.Context, instructorID uuid.UUID) (PublicTeacherCourseStats, error)
	ListPublishedCourses(ctx context.Context, instructorID uuid.UUID) ([]model.Course, error)
}

type PublicTeacherProfileRepository struct {
	db *gorm.DB
}

func NewPublicTeacherProfileRepository(db *gorm.DB) *PublicTeacherProfileRepository {
	return &PublicTeacherProfileRepository{db: db}
}

func (r *PublicTeacherProfileRepository) GetTeacher(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Where("users.id = ? AND users.is_active = ?", id, true).
		Where(`EXISTS (
			SELECT 1 FROM user_system_roles usr
			JOIN system_roles sr ON sr.id = usr.system_role_id AND sr.deleted_at IS NULL
			WHERE usr.user_id = users.id AND usr.deleted_at IS NULL AND usr.status = ? AND sr.name = ?)`,
			model.UserSystemRoleStatusActive, "TEACHER").
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *PublicTeacherProfileRepository) GetApprovedProfile(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error) {
	var p model.TeacherProfile
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND approval_status = ?", userID, model.TeacherApprovalApproved).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

func (r *PublicTeacherProfileRepository) PublishedCourseStats(ctx context.Context, instructorID uuid.UUID) (PublicTeacherCourseStats, error) {
	var s PublicTeacherCourseStats
	err := r.db.WithContext(ctx).Model(&model.Course{}).
		Select(`COUNT(*) AS course_count,
			COALESCE(SUM(total_students), 0) AS student_count,
			COALESCE(SUM(total_reviews), 0) AS review_count,
			COALESCE(SUM(average_rating * total_reviews), 0) AS rating_points`).
		Where("instructor_id = ? AND status = ?", instructorID, model.CourseStatusPublished).
		Scan(&s).Error
	return s, err
}

func (r *PublicTeacherProfileRepository) ListPublishedCourses(ctx context.Context, instructorID uuid.UUID) ([]model.Course, error) {
	var courses []model.Course
	err := r.db.WithContext(ctx).
		Where("instructor_id = ? AND status = ?", instructorID, model.CourseStatusPublished).
		Order("published_at DESC NULLS LAST, created_at DESC").
		Limit(maxPublicTeacherCourses).
		Find(&courses).Error
	return courses, err
}
