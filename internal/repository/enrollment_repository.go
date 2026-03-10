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
	GetByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error)
	GetDetailByID(ctx context.Context, id uuid.UUID) (*model.Enrollment, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Enrollment, int64, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, enrollment *model.Enrollment) error

	// Lookup
	GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error)

	// LessonProgress
	UpsertLessonProgress(ctx context.Context, progress *model.LessonProgress) error
	GetLessonProgress(ctx context.Context, userID, lessonID uuid.UUID) (*model.LessonProgress, error)
	CountCompletedMandatory(ctx context.Context, enrollmentID uuid.UUID) (int64, error)
	CountTotalMandatory(ctx context.Context, courseID uuid.UUID) (int64, error)
	UpdateEnrollmentProgress(ctx context.Context, enrollmentID uuid.UUID, progress decimal.Decimal) error
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
		Order("enrolled_at DESC").
		Find(&enrollments).Error; err != nil {
		return nil, 0, err
	}

	return enrollments, total, nil
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
