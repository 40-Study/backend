package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type AssignmentRepositoryInterface interface {
	Create(ctx context.Context, assignment *model.Assignment) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Assignment, error)
	GetByIDWithSession(ctx context.Context, id uuid.UUID) (*model.Assignment, error)
	GetByIDWithTestCases(ctx context.Context, id uuid.UUID) (*model.Assignment, error)
	GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Assignment, int64, error)
	GetPublishedBySession(ctx context.Context, sessionID uuid.UUID) ([]model.Assignment, error)
	Update(ctx context.Context, assignment *model.Assignment) error
	Delete(ctx context.Context, id uuid.UUID) error
	Publish(ctx context.Context, id uuid.UUID) error
	Unpublish(ctx context.Context, id uuid.UUID) error
}

type AssignmentRepository struct {
	db *gorm.DB
}

func NewAssignmentRepository(db *gorm.DB) *AssignmentRepository {
	return &AssignmentRepository{db: db}
}

func (r *AssignmentRepository) Create(ctx context.Context, assignment *model.Assignment) error {
	return r.db.WithContext(ctx).Create(assignment).Error
}

func (r *AssignmentRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Assignment, error) {
	var assignment model.Assignment
	err := r.db.WithContext(ctx).First(&assignment, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &assignment, nil
}

func (r *AssignmentRepository) GetByIDWithSession(ctx context.Context, id uuid.UUID) (*model.Assignment, error) {
	var assignment model.Assignment
	err := r.db.WithContext(ctx).
		Preload("Session").
		First(&assignment, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &assignment, nil
}

func (r *AssignmentRepository) GetByIDWithTestCases(ctx context.Context, id uuid.UUID) (*model.Assignment, error) {
	var assignment model.Assignment
	err := r.db.WithContext(ctx).
		Preload("TestCases", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		First(&assignment, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &assignment, nil
}

func (r *AssignmentRepository) GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Assignment, int64, error) {
	var assignments []model.Assignment
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Assignment{}).Where("session_id = ?", sessionID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := utils.ApplyPagination(query, page, pageSize).
		Order("created_at DESC").
		Find(&assignments).Error

	return assignments, total, err
}

func (r *AssignmentRepository) GetPublishedBySession(ctx context.Context, sessionID uuid.UUID) ([]model.Assignment, error) {
	var assignments []model.Assignment
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND is_published = ?", sessionID, true).
		Order("published_at DESC").
		Find(&assignments).Error
	return assignments, err
}

func (r *AssignmentRepository) Update(ctx context.Context, assignment *model.Assignment) error {
	return r.db.WithContext(ctx).Save(assignment).Error
}

func (r *AssignmentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Assignment{}, "id = ?", id).Error
}

func (r *AssignmentRepository) Publish(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.Assignment{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_published": true,
			"published_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}

func (r *AssignmentRepository) Unpublish(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.Assignment{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_published": false,
			"published_at": nil,
		}).Error
}

type TestCaseRepositoryInterface interface {
	Create(ctx context.Context, testCase *model.TestCase) error
	CreateBatch(ctx context.Context, testCases []model.TestCase) error
	GetByAssignment(ctx context.Context, assignmentID uuid.UUID) ([]model.TestCase, error)
	GetNonHiddenByAssignment(ctx context.Context, assignmentID uuid.UUID) ([]model.TestCase, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByAssignment(ctx context.Context, assignmentID uuid.UUID) error
}

type TestCaseRepository struct {
	db *gorm.DB
}

func NewTestCaseRepository(db *gorm.DB) *TestCaseRepository {
	return &TestCaseRepository{db: db}
}

func (r *TestCaseRepository) Create(ctx context.Context, testCase *model.TestCase) error {
	return r.db.WithContext(ctx).Create(testCase).Error
}

func (r *TestCaseRepository) CreateBatch(ctx context.Context, testCases []model.TestCase) error {
	if len(testCases) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&testCases).Error
}

func (r *TestCaseRepository) GetByAssignment(ctx context.Context, assignmentID uuid.UUID) ([]model.TestCase, error) {
	var testCases []model.TestCase
	err := r.db.WithContext(ctx).
		Where("assignment_id = ?", assignmentID).
		Order("display_order ASC").
		Find(&testCases).Error
	return testCases, err
}

func (r *TestCaseRepository) GetNonHiddenByAssignment(ctx context.Context, assignmentID uuid.UUID) ([]model.TestCase, error) {
	var testCases []model.TestCase
	err := r.db.WithContext(ctx).
		Where("assignment_id = ? AND is_hidden = ?", assignmentID, false).
		Order("display_order ASC").
		Find(&testCases).Error
	return testCases, err
}

func (r *TestCaseRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.TestCase{}, "id = ?", id).Error
}

func (r *TestCaseRepository) DeleteByAssignment(ctx context.Context, assignmentID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("assignment_id = ?", assignmentID).Delete(&model.TestCase{}).Error
}
