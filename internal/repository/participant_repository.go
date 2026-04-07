package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type ParticipantRepositoryInterface interface {
	Create(ctx context.Context, participant *model.Participant) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Participant, error)
	GetBySessionAndUser(ctx context.Context, sessionID, userID uuid.UUID) (*model.Participant, error)
	GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error)
	GetActiveBySession(ctx context.Context, sessionID uuid.UUID) ([]model.Participant, error)
	Update(ctx context.Context, participant *model.Participant) error
	UpdateRole(ctx context.Context, id uuid.UUID, role model.ParticipantRole) error
	SetLeft(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountBySession(ctx context.Context, sessionID uuid.UUID) (int64, error)
	CountActiveBySession(ctx context.Context, sessionID uuid.UUID) (int64, error)
	FindActiveByParentAndStudent(ctx context.Context, parentID, studentID uuid.UUID) (*model.ParentStudentRelation, error) // tự động hết hạn lời mời cũ (status chuyển sang "expired")
	CountActiveByStudentID(ctx context.Context, studentID uuid.UUID) (int64, error)
}

type ParticipantRepository struct {
	db *gorm.DB
}

func NewParticipantRepository(db *gorm.DB) *ParticipantRepository {
	return &ParticipantRepository{db: db}
}

func (r *ParticipantRepository) Create(ctx context.Context, participant *model.Participant) error {
	return r.db.WithContext(ctx).Create(participant).Error
}

func (r *ParticipantRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Participant, error) {
	var participant model.Participant
	err := r.db.WithContext(ctx).First(&participant, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &participant, nil
}

func (r *ParticipantRepository) GetBySessionAndUser(ctx context.Context, sessionID, userID uuid.UUID) (*model.Participant, error) {
	var participant model.Participant
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND is_active = ?", sessionID, userID, true).
		First(&participant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &participant, nil
}

func (r *ParticipantRepository) GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error) {
	var participants []model.Participant
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Participant{}).Where("session_id = ?", sessionID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := utils.ApplyPagination(query, page, pageSize).
		Preload("User").
		Order("joined_at DESC").
		Find(&participants).Error

	return participants, total, err
}

func (r *ParticipantRepository) GetActiveBySession(ctx context.Context, sessionID uuid.UUID) ([]model.Participant, error) {
	var participants []model.Participant
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND is_active = ?", sessionID, true).
		Find(&participants).Error
	return participants, err
}

func (r *ParticipantRepository) Update(ctx context.Context, participant *model.Participant) error {
	return r.db.WithContext(ctx).Save(participant).Error
}

func (r *ParticipantRepository) UpdateRole(ctx context.Context, id uuid.UUID, role model.ParticipantRole) error {
	return r.db.WithContext(ctx).
		Model(&model.Participant{}).
		Where("id = ?", id).
		Update("role", role).Error
}

func (r *ParticipantRepository) SetLeft(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.Participant{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_active": false,
			"left_at":   gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}

func (r *ParticipantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Participant{}, "id = ?", id).Error
}

func (r *ParticipantRepository) CountBySession(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Participant{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error
	return count, err
}

func (r *ParticipantRepository) CountActiveBySession(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Participant{}).
		Where("session_id = ? AND is_active = ?", sessionID, true).
		Count(&count).Error
	return count, err
}

func (r *ParticipantRepository) FindActiveByParentAndStudent(ctx context.Context, parentID, studentID uuid.UUID) (*model.ParentStudentRelation, error) {
	var relation model.ParentStudentRelation
	err := r.db.WithContext(ctx).
		Where("parent_user_id = ? AND student_user_id = ? AND status = ?", parentID, studentID, model.ParentStudentRelationStatusActive).
		First(&relation).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &relation, nil
}

func (r *ParticipantRepository) CountActiveByStudentID(ctx context.Context, studentID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.ParentStudentRelation{}).
		Where("student_user_id = ? AND status = ?", studentID, model.ParentStudentRelationStatusActive).
		Count(&count).Error
	return count, err
}
