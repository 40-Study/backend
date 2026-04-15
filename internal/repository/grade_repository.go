package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type GradeRepositoryInterface interface {
	// GradeColumn
	CreateGradeColumn(ctx context.Context, col *model.GradeColumn) error
	GetGradeColumnByID(ctx context.Context, id uuid.UUID) (*model.GradeColumn, error)
	GetGradeColumnsByClassID(ctx context.Context, classID uuid.UUID) ([]model.GradeColumn, error)
	UpdateGradeColumn(ctx context.Context, col *model.GradeColumn) error
	DeleteGradeColumn(ctx context.Context, id uuid.UUID) error
	ReorderGradeColumns(ctx context.Context, classID uuid.UUID, columnIDs []uuid.UUID) error

	// Grade
	CreateGrade(ctx context.Context, grade *model.Grade) error
	BulkCreateGrades(ctx context.Context, grades []model.Grade) error
	GetGradeByID(ctx context.Context, id uuid.UUID) (*model.Grade, error)
	GetGradesByClassID(ctx context.Context, classID uuid.UUID) ([]model.Grade, error)
	GetGradesByStudentAndClass(ctx context.Context, studentID, classID uuid.UUID) ([]model.Grade, error)
	GetGradesByStudentID(ctx context.Context, studentID uuid.UUID) ([]model.Grade, error)
	UpdateGrade(ctx context.Context, grade *model.Grade) error
	DeleteGrade(ctx context.Context, id uuid.UUID) error

	// FinalGrade
	CreateFinalGrade(ctx context.Context, fg *model.FinalGrade) error
	UpsertFinalGrade(ctx context.Context, fg *model.FinalGrade) error
	GetFinalGradesByClassID(ctx context.Context, classID uuid.UUID) ([]model.FinalGrade, error)
	GetFinalGradeByID(ctx context.Context, id uuid.UUID) (*model.FinalGrade, error)
	UpdateFinalGrade(ctx context.Context, fg *model.FinalGrade) error
	GetFinalGradeByStudentAndClass(ctx context.Context, studentID, classID uuid.UUID) (*model.FinalGrade, error)
	FinalizeFinalGrades(ctx context.Context, classID, finalizedBy uuid.UUID) error
}

type GradeRepository struct {
	db *gorm.DB
}

func NewGradeRepository(db *gorm.DB) *GradeRepository {
	return &GradeRepository{db: db}
}

// ============================================================================
// GRADE COLUMN
// ============================================================================

func (r *GradeRepository) CreateGradeColumn(ctx context.Context, col *model.GradeColumn) error {
	return r.db.WithContext(ctx).Create(col).Error
}

func (r *GradeRepository) GetGradeColumnByID(ctx context.Context, id uuid.UUID) (*model.GradeColumn, error) {
	var col model.GradeColumn
	err := r.db.WithContext(ctx).First(&col, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &col, nil
}

func (r *GradeRepository) GetGradeColumnsByClassID(ctx context.Context, classID uuid.UUID) ([]model.GradeColumn, error) {
	var cols []model.GradeColumn
	err := r.db.WithContext(ctx).
		Where("class_id = ?", classID).
		Order("display_order ASC").
		Find(&cols).Error
	return cols, err
}

func (r *GradeRepository) UpdateGradeColumn(ctx context.Context, col *model.GradeColumn) error {
	return r.db.WithContext(ctx).Save(col).Error
}

func (r *GradeRepository) DeleteGradeColumn(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.GradeColumn{}, "id = ?", id).Error
}

func (r *GradeRepository) ReorderGradeColumns(ctx context.Context, classID uuid.UUID, columnIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, colID := range columnIDs {
			if err := tx.Model(&model.GradeColumn{}).
				Where("id = ? AND class_id = ?", colID, classID).
				Update("display_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ============================================================================
// GRADE
// ============================================================================

func (r *GradeRepository) CreateGrade(ctx context.Context, grade *model.Grade) error {
	return r.db.WithContext(ctx).Create(grade).Error
}

func (r *GradeRepository) BulkCreateGrades(ctx context.Context, grades []model.Grade) error {
	return r.db.WithContext(ctx).Create(&grades).Error
}

func (r *GradeRepository) GetGradeByID(ctx context.Context, id uuid.UUID) (*model.Grade, error) {
	var grade model.Grade
	err := r.db.WithContext(ctx).
		Preload("Student").
		Preload("Grader").
		First(&grade, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &grade, nil
}

func (r *GradeRepository) GetGradesByClassID(ctx context.Context, classID uuid.UUID) ([]model.Grade, error) {
	var grades []model.Grade
	err := r.db.WithContext(ctx).
		Preload("Student").
		Where("class_id = ?", classID).
		Order("student_id ASC, grade_type ASC").
		Find(&grades).Error
	return grades, err
}

func (r *GradeRepository) GetGradesByStudentAndClass(ctx context.Context, studentID, classID uuid.UUID) ([]model.Grade, error) {
	var grades []model.Grade
	err := r.db.WithContext(ctx).
		Preload("Student").
		Where("student_id = ? AND class_id = ?", studentID, classID).
		Order("grade_type ASC, created_at ASC").
		Find(&grades).Error
	return grades, err
}

func (r *GradeRepository) GetGradesByStudentID(ctx context.Context, studentID uuid.UUID) ([]model.Grade, error) {
	var grades []model.Grade
	err := r.db.WithContext(ctx).
		Preload("Class").
		Where("student_id = ?", studentID).
		Order("created_at DESC").
		Find(&grades).Error
	return grades, err
}

func (r *GradeRepository) UpdateGrade(ctx context.Context, grade *model.Grade) error {
	return r.db.WithContext(ctx).Save(grade).Error
}

func (r *GradeRepository) DeleteGrade(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Grade{}, "id = ?", id).Error
}

// ============================================================================
// FINAL GRADE
// ============================================================================

func (r *GradeRepository) CreateFinalGrade(ctx context.Context, fg *model.FinalGrade) error {
	return r.db.WithContext(ctx).Create(fg).Error
}

func (r *GradeRepository) UpsertFinalGrade(ctx context.Context, fg *model.FinalGrade) error {
	return r.db.WithContext(ctx).
		Where("student_id = ? AND class_id = ?", fg.StudentID, fg.ClassID).
		Assign(model.FinalGrade{
			WeightedAverage: fg.WeightedAverage,
			LetterGrade:     fg.LetterGrade,
			GPA:             fg.GPA,
			Rank:            fg.Rank,
			Status:          fg.Status,
			CalculatedAt:    fg.CalculatedAt,
		}).
		FirstOrCreate(fg).Error
}

func (r *GradeRepository) GetFinalGradesByClassID(ctx context.Context, classID uuid.UUID) ([]model.FinalGrade, error) {
	var fgs []model.FinalGrade
	err := r.db.WithContext(ctx).
		Preload("Student").
		Where("class_id = ?", classID).
		Order("rank ASC NULLS LAST").
		Find(&fgs).Error
	return fgs, err
}

func (r *GradeRepository) GetFinalGradeByID(ctx context.Context, id uuid.UUID) (*model.FinalGrade, error) {
	var fg model.FinalGrade
	err := r.db.WithContext(ctx).
		Preload("Student").
		First(&fg, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &fg, nil
}

func (r *GradeRepository) UpdateFinalGrade(ctx context.Context, fg *model.FinalGrade) error {
	return r.db.WithContext(ctx).Save(fg).Error
}

func (r *GradeRepository) GetFinalGradeByStudentAndClass(ctx context.Context, studentID, classID uuid.UUID) (*model.FinalGrade, error) {
	var fg model.FinalGrade
	err := r.db.WithContext(ctx).
		Where("student_id = ? AND class_id = ?", studentID, classID).
		First(&fg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &fg, nil
}

func (r *GradeRepository) FinalizeFinalGrades(ctx context.Context, classID, finalizedBy uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.FinalGrade{}).
		Where("class_id = ? AND finalized_at IS NULL", classID).
		Updates(map[string]interface{}{
			"finalized_at": gorm.Expr("NOW()"),
			"finalized_by": finalizedBy,
			"status":       "passed",
		}).Error
}
