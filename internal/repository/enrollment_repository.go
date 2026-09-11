package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type EnrollmentRepositoryInterface interface {
	Create(ctx context.Context, enrollment *model.Enrollment) error
	GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error)
	// GetByUserAndCourseUnscoped giống GetByUserAndCourse nhưng bao gồm cả bản ghi đã soft-delete
	// (dùng để phát hiện re-enroll sau khi Unenroll — xem C-06 audit 260909).
	GetByUserAndCourseUnscoped(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error)
	// Restore khôi phục một enrollment đã soft-delete (deleted_at = NULL).
	Restore(ctx context.Context, id uuid.UUID) error
	// RestoreAndReactivate (H-02, review vòng 1): gộp restore (deleted_at = NULL) và reset các
	// field tiến trình học vào ĐÚNG MỘT câu UPDATE, tránh lỗi Save() ghi đè deleted_at cũ khi
	// re-enroll — xem comment tại EnrollmentService.Enroll để biết bối cảnh đầy đủ.
	RestoreAndReactivate(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error)
	GetDetailByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error)
	GetByCourseID(ctx context.Context, courseID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error)
	GetByCourseIDIncludeDeleted(ctx context.Context, courseID uuid.UUID) ([]model.Enrollment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, enrollment *model.Enrollment) error

	// Lookup
	GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error)
	GetEnrolledUserIDsByCourseID(ctx context.Context, courseID uuid.UUID) ([]uuid.UUID, error)

	// LessonProgress
	UpsertLessonProgress(ctx context.Context, progress *model.LessonProgress) error
	GetLessonProgress(ctx context.Context, userID, lessonID uuid.UUID) (*model.LessonProgress, error)
	CountCompletedMandatory(ctx context.Context, enrollmentID uuid.UUID) (int64, error)
	CountTotalMandatory(ctx context.Context, courseID uuid.UUID) (int64, error)
	UpdateEnrollmentProgress(ctx context.Context, enrollmentID uuid.UUID, progress decimal.Decimal) error
	// SumWatchedSecondsByEnrollmentIDs cong don video_watched_seconds theo tung enrollment
	// bang DUNG MOT cau GROUP BY (tranh N+1 khi liet ke danh sach ghi danh).
	SumWatchedSecondsByEnrollmentIDs(ctx context.Context, enrollmentIDs []uuid.UUID) (map[uuid.UUID]int, error)
}

type EnrollmentRepository struct {
	db *gorm.DB
}

func NewEnrollmentRepository(db *gorm.DB) *EnrollmentRepository {
	return &EnrollmentRepository{db: db}
}

func (r *EnrollmentRepository) Create(ctx context.Context, enrollment *model.Enrollment) error {
	return r.db.WithContext(ctx).Create(enrollment).Error
}

func (r *EnrollmentRepository) GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	var enrollment model.Enrollment
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND course_id = ?", userID, courseID).
		First(&enrollment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &enrollment, nil
}

// GetByUserAndCourseUnscoped tìm enrollment kể cả đã soft-delete (Unscoped) — dùng để phân
// biệt "chưa từng enroll" với "đã unenroll trước đó" khi xử lý re-enroll (C-06).
func (r *EnrollmentRepository) GetByUserAndCourseUnscoped(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	var enrollment model.Enrollment
	err := r.db.WithContext(ctx).
		Unscoped().
		Where("user_id = ? AND course_id = ?", userID, courseID).
		First(&enrollment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &enrollment, nil
}

// Restore khôi phục enrollment đã soft-delete (deleted_at = NULL).
func (r *EnrollmentRepository) Restore(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Model(&model.Enrollment{}).Where("id = ?", id).Update("deleted_at", nil).Error
}

// RestoreAndReactivate (H-02, review vòng 1): trước đây service gọi Restore() rồi gọi tiếp
// Update() (db.Save()) trên struct đã load TRƯỚC Restore — struct đó vẫn giữ DeletedAt cũ
// trong bộ nhớ, nên Save() ghi đè lại đúng giá trị deleted_at vừa xóa, vô hiệu hóa Restore().
// Gộp restore + set field vào MỘT lệnh Updates() duy nhất (map, không qua struct) để tránh
// hoàn toàn vấn đề stale-in-memory-field.
// buildRestoreAndReactivateQuery (M2-05, review vòng 3): tách phần XÂY câu UPDATE ra khỏi phần
// đọc .Error, để test DryRun (enrollment_repository_test.go) gọi được ĐÚNG hàm sản xuất thật
// thay vì hand-roll lại câu query trong test — xóa/sửa sai hàm này sẽ làm test đỏ.
func (r *EnrollmentRepository) buildRestoreAndReactivateQuery(ctx context.Context, id uuid.UUID, updates map[string]interface{}) *gorm.DB {
	updates["deleted_at"] = nil
	return r.db.WithContext(ctx).Unscoped().Model(&model.Enrollment{}).Where("id = ?", id).Updates(updates)
}

func (r *EnrollmentRepository) RestoreAndReactivate(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	return r.buildRestoreAndReactivateQuery(ctx, id, updates).Error
}

func (r *EnrollmentRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error) {
	var enrollment model.Enrollment
	err := r.db.WithContext(ctx).
		Preload("Course").
		Where("id = ?", id).
		First(&enrollment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &enrollment, nil
}

func (r *EnrollmentRepository) GetDetailByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error) {
	var enrollment model.Enrollment
	err := r.db.WithContext(ctx).
		Preload("Course").
		Preload("LessonProgress").
		Where("id = ?", id).
		First(&enrollment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &enrollment, nil
}

func (r *EnrollmentRepository) GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error) {
	var enrollments []model.Enrollment
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Enrollment{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Preload("Course").
		Preload("Course.Category").
		Preload("Course.Instructor").
		Order("enrolled_at DESC").
		Find(&enrollments).Error; err != nil {
		return nil, 0, err
	}

	return enrollments, total, nil
}

// SumWatchedSecondsByEnrollmentIDs - xem ghi chu tren interface.
func (r *EnrollmentRepository) SumWatchedSecondsByEnrollmentIDs(ctx context.Context, enrollmentIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	totals := make(map[uuid.UUID]int, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return totals, nil
	}

	var rows []struct {
		EnrollmentID uuid.UUID
		Total        int
	}
	if err := r.db.WithContext(ctx).
		Model(&model.LessonProgress{}).
		Select("enrollment_id, COALESCE(SUM(video_watched_seconds), 0) AS total").
		Where("enrollment_id IN ?", enrollmentIDs).
		Group("enrollment_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		totals[row.EnrollmentID] = row.Total
	}
	return totals, nil
}

func (r *EnrollmentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Enrollment{}, "id = ?", id).Error
}

func (r *EnrollmentRepository) Update(ctx context.Context, enrollment *model.Enrollment) error {
	return r.db.WithContext(ctx).Save(enrollment).Error
}

// Lookup

func (r *EnrollmentRepository) GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error) {
	var result struct {
		CourseID uuid.UUID
	}
	err := r.db.WithContext(ctx).
		Model(&model.Section{}).
		Select("sections.course_id").
		Joins("JOIN lessons ON lessons.section_id = sections.id").
		Where("lessons.id = ?", lessonID).
		Scan(&result).Error
	if err != nil {
		return uuid.Nil, err
	}
	if result.CourseID == uuid.Nil {
		return uuid.Nil, errors.New("lesson not found in any course")
	}
	return result.CourseID, nil
}

func (r *EnrollmentRepository) GetEnrolledUserIDsByCourseID(ctx context.Context, courseID uuid.UUID) ([]uuid.UUID, error) {
	var userIDs []uuid.UUID
	err := r.db.WithContext(ctx).
		Model(&model.Enrollment{}).
		Where("course_id = ?", courseID).
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

// GetByCourseID returns all enrollments for a course (for instructor view)
func (r *EnrollmentRepository) GetByCourseID(ctx context.Context, courseID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error) {
	var enrollments []model.Enrollment
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Enrollment{}).Where("course_id = ?", courseID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Preload("User").
		Order("enrolled_at DESC").
		Find(&enrollments).Error; err != nil {
		return nil, 0, err
	}

	return enrollments, total, nil
}

// GetByCourseIDIncludeDeleted returns all enrollments including soft-deleted (for debugging)
func (r *EnrollmentRepository) GetByCourseIDIncludeDeleted(ctx context.Context, courseID uuid.UUID) ([]model.Enrollment, error) {
	var enrollments []model.Enrollment
	err := r.db.WithContext(ctx).
		Unscoped(). // Include soft-deleted records
		Where("course_id = ?", courseID).
		Preload("User").
		Find(&enrollments).Error
	return enrollments, err
}

// LessonProgress

func (r *EnrollmentRepository) UpsertLessonProgress(ctx context.Context, progress *model.LessonProgress) error {
	return r.db.WithContext(ctx).Save(progress).Error
}

func (r *EnrollmentRepository) GetLessonProgress(ctx context.Context, userID, lessonID uuid.UUID) (*model.LessonProgress, error) {
	var progress model.LessonProgress
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND lesson_id = ?", userID, lessonID).
		First(&progress).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &progress, nil
}

func (r *EnrollmentRepository) CountCompletedMandatory(ctx context.Context, enrollmentID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.LessonProgress{}).
		Joins("JOIN lessons ON lessons.id = lesson_progress.lesson_id").
		Where("lesson_progress.enrollment_id = ? AND lesson_progress.status = 'completed' AND lessons.is_mandatory = true", enrollmentID).
		Count(&count).Error
	return count, err
}

func (r *EnrollmentRepository) CountTotalMandatory(ctx context.Context, courseID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Joins("JOIN sections ON sections.id = lessons.section_id").
		Where("sections.course_id = ? AND lessons.is_mandatory = true", courseID).
		Count(&count).Error
	return count, err
}

func (r *EnrollmentRepository) UpdateEnrollmentProgress(ctx context.Context, enrollmentID uuid.UUID, progress decimal.Decimal) error {
	return r.db.WithContext(ctx).
		Model(&model.Enrollment{}).
		Where("id = ?", enrollmentID).
		Update("progress_percentage", progress).Error
}
