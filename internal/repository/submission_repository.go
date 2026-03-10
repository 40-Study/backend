package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type SubmissionRepositoryInterface interface {
	Create(ctx context.Context, submission *model.Submission) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Submission, error)
	GetByAssignment(ctx context.Context, assignmentID uuid.UUID, page, pageSize int) ([]model.Submission, int64, error)
	GetByUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Submission, int64, error)
	GetByAssignmentAndUser(ctx context.Context, assignmentID, userID uuid.UUID) ([]model.Submission, error)
	Update(ctx context.Context, submission *model.Submission) error
	UpdateVerdict(ctx context.Context, id uuid.UUID, verdict model.SubmissionVerdict, execTime, memoryUsed, passed, total int) error
}

type SubmissionRepository struct {
	db *gorm.DB
}

func NewSubmissionRepository(db *gorm.DB) *SubmissionRepository {
	return &SubmissionRepository{db: db}
}

func (r *SubmissionRepository) Create(ctx context.Context, submission *model.Submission) error {
	return r.db.WithContext(ctx).Create(submission).Error
}

func (r *SubmissionRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Submission, error) {
	var submission model.Submission
	err := r.db.WithContext(ctx).
		Preload("Assignment").
		First(&submission, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &submission, nil
}

func (r *SubmissionRepository) GetByAssignment(ctx context.Context, assignmentID uuid.UUID, page, pageSize int) ([]model.Submission, int64, error) {
	var submissions []model.Submission
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Submission{}).Where("assignment_id = ?", assignmentID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := utils.ApplyPagination(query, page, pageSize).
		Preload("User").
		Order("created_at DESC").
		Find(&submissions).Error

	return submissions, total, err
}

func (r *SubmissionRepository) GetByUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Submission, int64, error) {
	var submissions []model.Submission
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Submission{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := utils.ApplyPagination(query, page, pageSize).
		Preload("Assignment").
		Order("created_at DESC").
		Find(&submissions).Error

	return submissions, total, err
}

func (r *SubmissionRepository) GetByAssignmentAndUser(ctx context.Context, assignmentID, userID uuid.UUID) ([]model.Submission, error) {
	var submissions []model.Submission
	err := r.db.WithContext(ctx).
		Where("assignment_id = ? AND user_id = ?", assignmentID, userID).
		Order("created_at DESC").
		Find(&submissions).Error
	return submissions, err
}

func (r *SubmissionRepository) Update(ctx context.Context, submission *model.Submission) error {
	return r.db.WithContext(ctx).Save(submission).Error
}

func (r *SubmissionRepository) UpdateVerdict(ctx context.Context, id uuid.UUID, verdict model.SubmissionVerdict, execTime, memoryUsed, passed, total int) error {
	return r.db.WithContext(ctx).
		Model(&model.Submission{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"verdict":           verdict,
			"execution_time":    execTime,
			"memory_used":       memoryUsed,
			"test_cases_passed": passed,
			"total_test_cases":  total,
		}).Error
}
