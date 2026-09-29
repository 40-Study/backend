package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type ExerciseRepositoryInterface interface {
	// CourseExercise
	CreateExercise(ctx context.Context, exercise *model.CourseExercise) error
	GetExerciseByID(ctx context.Context, id uuid.UUID) (*model.CourseExercise, error)
	GetExerciseWithTestCases(ctx context.Context, id uuid.UUID) (*model.CourseExercise, error)
	ListExercises(ctx context.Context, page, pageSize int) ([]model.CourseExercise, int64, error)
	UpdateExercise(ctx context.Context, exercise *model.CourseExercise) error
	DeleteExercise(ctx context.Context, id uuid.UUID) error
	CanManage(ctx context.Context, exerciseID, userID uuid.UUID) (bool, error)

	// ExerciseTestCase
	CreateTestCase(ctx context.Context, tc *model.ExerciseTestCase) error
	BulkCreateTestCases(ctx context.Context, tcs []model.ExerciseTestCase) error
	GetTestCasesByExerciseID(ctx context.Context, exerciseID uuid.UUID) ([]model.ExerciseTestCase, error)
	DeleteTestCase(ctx context.Context, exerciseID, id uuid.UUID) error
	DeleteTestCasesByExerciseID(ctx context.Context, exerciseID uuid.UUID) error

	// ExerciseSubmission
	CreateSubmission(ctx context.Context, sub *model.ExerciseSubmission) error
	GetSubmissionByID(ctx context.Context, id uuid.UUID) (*model.ExerciseSubmission, error)
	GetSubmissionsByExerciseAndUser(ctx context.Context, exerciseID, userID uuid.UUID, page, pageSize int) ([]model.ExerciseSubmission, int64, error)
	GetSubmissionsByExercise(ctx context.Context, exerciseID uuid.UUID, page, pageSize int) ([]model.ExerciseSubmission, int64, error)
	GetLatestSubmission(ctx context.Context, exerciseID, userID uuid.UUID) (*model.ExerciseSubmission, error)
	UpdateSubmission(ctx context.Context, sub *model.ExerciseSubmission) error

	// ContentProgress
	GetContentProgress(ctx context.Context, userID, lessonContentID uuid.UUID) (*model.ContentProgress, error)
	UpsertContentProgress(ctx context.Context, progress *model.ContentProgress) error
}

type ExerciseRepository struct {
	db *gorm.DB
}

func NewExerciseRepository(db *gorm.DB) *ExerciseRepository {
	return &ExerciseRepository{db: db}
}

// ============================================================================
// COURSE EXERCISE
// ============================================================================

func (r *ExerciseRepository) CreateExercise(ctx context.Context, exercise *model.CourseExercise) error {
	return r.db.WithContext(ctx).Create(exercise).Error
}

func (r *ExerciseRepository) GetExerciseByID(ctx context.Context, id uuid.UUID) (*model.CourseExercise, error) {
	var ex model.CourseExercise
	err := r.db.WithContext(ctx).First(&ex, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ex, nil
}

func (r *ExerciseRepository) GetExerciseWithTestCases(ctx context.Context, id uuid.UUID) (*model.CourseExercise, error) {
	var ex model.CourseExercise
	err := r.db.WithContext(ctx).
		Preload("TestCases", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		First(&ex, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ex, nil
}

func (r *ExerciseRepository) ListExercises(ctx context.Context, page, pageSize int) ([]model.CourseExercise, int64, error) {
	var exercises []model.CourseExercise
	var total int64

	query := r.db.WithContext(ctx).Model(&model.CourseExercise{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&exercises).Error
	return exercises, total, err
}

func (r *ExerciseRepository) UpdateExercise(ctx context.Context, exercise *model.CourseExercise) error {
	return r.db.WithContext(ctx).Save(exercise).Error
}

func (r *ExerciseRepository) DeleteExercise(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.CourseExercise{}, "id = ?", id).Error
}

// ============================================================================
// EXERCISE TEST CASE
// ============================================================================

func (r *ExerciseRepository) CreateTestCase(ctx context.Context, tc *model.ExerciseTestCase) error {
	return r.db.WithContext(ctx).Create(tc).Error
}

func (r *ExerciseRepository) BulkCreateTestCases(ctx context.Context, tcs []model.ExerciseTestCase) error {
	return r.db.WithContext(ctx).Create(&tcs).Error
}

func (r *ExerciseRepository) GetTestCasesByExerciseID(ctx context.Context, exerciseID uuid.UUID) ([]model.ExerciseTestCase, error) {
	var tcs []model.ExerciseTestCase
	err := r.db.WithContext(ctx).
		Where("exercise_id = ?", exerciseID).
		Order("display_order ASC").
		Find(&tcs).Error
	return tcs, err
}

// DeleteTestCase chỉ xoá test case thuộc đúng bài tập (S2), tránh xoá chéo bài tập khác qua :id giả.
func (r *ExerciseRepository) DeleteTestCase(ctx context.Context, exerciseID, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.ExerciseTestCase{}, "id = ? AND exercise_id = ?", id, exerciseID).Error
}

func (r *ExerciseRepository) DeleteTestCasesByExerciseID(ctx context.Context, exerciseID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("exercise_id = ?", exerciseID).Delete(&model.ExerciseTestCase{}).Error
}

// ============================================================================
// EXERCISE SUBMISSION
// ============================================================================

func (r *ExerciseRepository) CreateSubmission(ctx context.Context, sub *model.ExerciseSubmission) error {
	return r.db.WithContext(ctx).Create(sub).Error
}

func (r *ExerciseRepository) GetSubmissionByID(ctx context.Context, id uuid.UUID) (*model.ExerciseSubmission, error) {
	var sub model.ExerciseSubmission
	err := r.db.WithContext(ctx).First(&sub, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sub, nil
}

func (r *ExerciseRepository) GetSubmissionsByExerciseAndUser(ctx context.Context, exerciseID, userID uuid.UUID, page, pageSize int) ([]model.ExerciseSubmission, int64, error) {
	var subs []model.ExerciseSubmission
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ExerciseSubmission{}).
		Where("exercise_id = ? AND user_id = ?", exerciseID, userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&subs).Error
	return subs, total, err
}

func (r *ExerciseRepository) GetSubmissionsByExercise(ctx context.Context, exerciseID uuid.UUID, page, pageSize int) ([]model.ExerciseSubmission, int64, error) {
	var subs []model.ExerciseSubmission
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ExerciseSubmission{}).Where("exercise_id = ?", exerciseID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&subs).Error
	return subs, total, err
}

func (r *ExerciseRepository) GetLatestSubmission(ctx context.Context, exerciseID, userID uuid.UUID) (*model.ExerciseSubmission, error) {
	var sub model.ExerciseSubmission
	err := r.db.WithContext(ctx).
		Where("exercise_id = ? AND user_id = ?", exerciseID, userID).
		Order("created_at DESC").
		First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sub, nil
}

func (r *ExerciseRepository) UpdateSubmission(ctx context.Context, sub *model.ExerciseSubmission) error {
	return r.db.WithContext(ctx).Save(sub).Error
}

// ============================================================================
// CONTENT PROGRESS
// ============================================================================

func (r *ExerciseRepository) GetContentProgress(ctx context.Context, userID, lessonContentID uuid.UUID) (*model.ContentProgress, error) {
	var cp model.ContentProgress
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND lesson_content_id = ?", userID, lessonContentID).
		First(&cp).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cp, nil
}

func (r *ExerciseRepository) UpsertContentProgress(ctx context.Context, progress *model.ContentProgress) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND lesson_content_id = ?", progress.UserID, progress.LessonContentID).
		Assign(*progress).
		FirstOrCreate(progress).Error
}

// CanManage (S2): true khi userID là người tạo bài tập (created_by) hoặc giảng viên chủ của MỘT
// khoá học đang gắn bài tập đó vào bài học (lesson_contents.exercise_id). Bài tập tạo trước khi có
// cột created_by và chưa gắn vào khoá nào thì chỉ admin quản lý được — chấp nhận, vì không có dữ
// liệu nào chứng minh ai là chủ.
func (r *ExerciseRepository) CanManage(ctx context.Context, exerciseID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).Raw(`
SELECT EXISTS (
  SELECT 1 FROM course_exercises e
   WHERE e.id = @eid AND e.deleted_at IS NULL
     AND (e.created_by = @uid
          OR EXISTS (SELECT 1
                       FROM lesson_contents lc
                       JOIN lessons l ON l.id = lc.lesson_id
                       JOIN sections s ON s.id = l.section_id AND s.deleted_at IS NULL
                       JOIN courses co ON co.id = s.course_id AND co.deleted_at IS NULL
                      WHERE lc.exercise_id = e.id AND co.instructor_id = @uid))
)`, sql.Named("eid", exerciseID), sql.Named("uid", userID)).Scan(&ok).Error
	return ok, err
}