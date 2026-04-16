package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// ============================================================================
// CONTEST REPOSITORY
// ============================================================================

type ContestRepositoryInterface interface {
	Create(ctx context.Context, contest *model.Contest) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Contest, error)
	GetBySlug(ctx context.Context, slug string) (*model.Contest, error)
	Update(ctx context.Context, contest *model.Contest) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, keyword string, status string, page, pageSize int) ([]model.Contest, int64, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]model.Contest, error)
	IncrementParticipantCount(ctx context.Context, contestID uuid.UUID) error
	UpdateStatus(ctx context.Context, contestID uuid.UUID, status model.ContestStatus) error
}

type ContestRepository struct {
	db *gorm.DB
}

func NewContestRepository(db *gorm.DB) *ContestRepository {
	return &ContestRepository{db: db}
}

func (r *ContestRepository) Create(ctx context.Context, contest *model.Contest) error {
	return r.db.WithContext(ctx).Create(contest).Error
}

func (r *ContestRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Contest, error) {
	var contest model.Contest
	err := r.db.WithContext(ctx).
		Preload("Problems").
		Preload("Participants").
		First(&contest, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &contest, nil
}

func (r *ContestRepository) GetBySlug(ctx context.Context, slug string) (*model.Contest, error) {
	var contest model.Contest
	err := r.db.WithContext(ctx).
		Preload("Problems").
		Preload("Participants").
		First(&contest, "slug = ?", slug).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &contest, nil
}

func (r *ContestRepository) Update(ctx context.Context, contest *model.Contest) error {
	return r.db.WithContext(ctx).Save(contest).Error
}

func (r *ContestRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Contest{}, "id = ?", id).Error
}

func (r *ContestRepository) List(ctx context.Context, keyword string, status string, page, pageSize int) ([]model.Contest, int64, error) {
	var contests []model.Contest
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Contest{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if keyword != "" {
		query = utils.ApplyKeywordSearch(query, keyword, "title", "description")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("created_at DESC").Find(&contests).Error
	return contests, total, err
}

func (r *ContestRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]model.Contest, error) {
	var contests []model.Contest

	err := r.db.WithContext(ctx).Model(&model.Contest{}).
		Joins("JOIN contest_participants ON contest_participants.contest_id = contests.id").
		Where("contest_participants.user_id = ?", userID).
		Where("contests.deleted_at IS NULL").
		Order("contests.created_at DESC").
		Find(&contests).Error
	return contests, err
}

func (r *ContestRepository) IncrementParticipantCount(ctx context.Context, contestID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.Contest{}).
		Where("id = ?", contestID).
		UpdateColumn("participant_count", gorm.Expr("participant_count + 1")).Error
}

func (r *ContestRepository) UpdateStatus(ctx context.Context, contestID uuid.UUID, status model.ContestStatus) error {
	return r.db.WithContext(ctx).Model(&model.Contest{}).
		Where("id = ?", contestID).
		UpdateColumn("status", status).Error
}

// ============================================================================
// CONTEST PROBLEM REPOSITORY
// ============================================================================

type ContestProblemRepositoryInterface interface {
	Create(ctx context.Context, problem *model.ContestProblem) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ContestProblem, error)
	Update(ctx context.Context, problem *model.ContestProblem) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByContestID(ctx context.Context, contestID uuid.UUID) ([]model.ContestProblem, error)
}

type ContestProblemRepository struct {
	db *gorm.DB
}

func NewContestProblemRepository(db *gorm.DB) *ContestProblemRepository {
	return &ContestProblemRepository{db: db}
}

func (r *ContestProblemRepository) Create(ctx context.Context, problem *model.ContestProblem) error {
	return r.db.WithContext(ctx).Create(problem).Error
}

func (r *ContestProblemRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ContestProblem, error) {
	var problem model.ContestProblem
	err := r.db.WithContext(ctx).First(&problem, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &problem, nil
}

func (r *ContestProblemRepository) Update(ctx context.Context, problem *model.ContestProblem) error {
	return r.db.WithContext(ctx).Save(problem).Error
}

func (r *ContestProblemRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.ContestProblem{}, "id = ?", id).Error
}

func (r *ContestProblemRepository) ListByContestID(ctx context.Context, contestID uuid.UUID) ([]model.ContestProblem, error) {
	var problems []model.ContestProblem
	err := r.db.WithContext(ctx).
		Where("contest_id = ?", contestID).
		Order("display_order ASC").
		Find(&problems).Error
	return problems, err
}

// ============================================================================
// CONTEST PARTICIPANT REPOSITORY
// ============================================================================

type ContestParticipantRepositoryInterface interface {
	Create(ctx context.Context, participant *model.ContestParticipant) error
	GetByContestAndUser(ctx context.Context, contestID, userID uuid.UUID) (*model.ContestParticipant, error)
	ListByContestID(ctx context.Context, contestID uuid.UUID, page, pageSize int) ([]model.ContestParticipant, int64, error)
	UpdateScore(ctx context.Context, participant *model.ContestParticipant) error
}

type ContestParticipantRepository struct {
	db *gorm.DB
}

func NewContestParticipantRepository(db *gorm.DB) *ContestParticipantRepository {
	return &ContestParticipantRepository{db: db}
}

func (r *ContestParticipantRepository) Create(ctx context.Context, participant *model.ContestParticipant) error {
	return r.db.WithContext(ctx).Create(participant).Error
}

func (r *ContestParticipantRepository) GetByContestAndUser(ctx context.Context, contestID, userID uuid.UUID) (*model.ContestParticipant, error) {
	var participant model.ContestParticipant
	err := r.db.WithContext(ctx).
		Where("contest_id = ? AND user_id = ?", contestID, userID).
		First(&participant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &participant, nil
}

func (r *ContestParticipantRepository) ListByContestID(ctx context.Context, contestID uuid.UUID, page, pageSize int) ([]model.ContestParticipant, int64, error) {
	var participants []model.ContestParticipant
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ContestParticipant{}).
		Where("contest_id = ?", contestID).
		Preload("User")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.Order("total_score DESC, created_at ASC").Find(&participants).Error
	return participants, total, err
}

func (r *ContestParticipantRepository) UpdateScore(ctx context.Context, participant *model.ContestParticipant) error {
	return r.db.WithContext(ctx).Save(participant).Error
}

// ============================================================================
// CONTEST SUBMISSION REPOSITORY
// ============================================================================

type ContestSubmissionRepositoryInterface interface {
	Create(ctx context.Context, submission *model.ContestSubmission) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ContestSubmission, error)
	ListByContestAndUser(ctx context.Context, contestID, userID uuid.UUID) ([]model.ContestSubmission, error)
	GetBestByContestProblemUser(ctx context.Context, contestID, problemID, userID uuid.UUID) (*model.ContestSubmission, error)
}

type ContestSubmissionRepository struct {
	db *gorm.DB
}

func NewContestSubmissionRepository(db *gorm.DB) *ContestSubmissionRepository {
	return &ContestSubmissionRepository{db: db}
}

func (r *ContestSubmissionRepository) Create(ctx context.Context, submission *model.ContestSubmission) error {
	return r.db.WithContext(ctx).Create(submission).Error
}

func (r *ContestSubmissionRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ContestSubmission, error) {
	var submission model.ContestSubmission
	err := r.db.WithContext(ctx).First(&submission, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &submission, nil
}

func (r *ContestSubmissionRepository) ListByContestAndUser(ctx context.Context, contestID, userID uuid.UUID) ([]model.ContestSubmission, error) {
	var submissions []model.ContestSubmission
	err := r.db.WithContext(ctx).
		Where("contest_id = ? AND user_id = ?", contestID, userID).
		Order("created_at DESC").
		Find(&submissions).Error
	return submissions, err
}

func (r *ContestSubmissionRepository) GetBestByContestProblemUser(ctx context.Context, contestID, problemID, userID uuid.UUID) (*model.ContestSubmission, error) {
	var submission model.ContestSubmission
	err := r.db.WithContext(ctx).
		Where("contest_id = ? AND problem_id = ? AND user_id = ?", contestID, problemID, userID).
		Order("score DESC").
		First(&submission).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &submission, nil
}
