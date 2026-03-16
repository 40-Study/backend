package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type ChatMessageRepositoryInterface interface {
	Create(ctx context.Context, message *model.ChatMessage) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ChatMessage, error)
	GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.ChatMessage, int64, error)
	GetBySessionWithReplies(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.ChatMessage, int64, error)
	Update(ctx context.Context, message *model.ChatMessage) error
	Delete(ctx context.Context, id uuid.UUID) error
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error
	Pin(ctx context.Context, id uuid.UUID) error
	UnPin(ctx context.Context, id uuid.UUID) error
	CountBySession(ctx context.Context, sessionID uuid.UUID) (int64, error)
}

type ChatMessageRepository struct {
	db *gorm.DB
}

func NewChatMessageRepository(db *gorm.DB) *ChatMessageRepository {
	return &ChatMessageRepository{db: db}
}

func (r *ChatMessageRepository) Create(ctx context.Context, message *model.ChatMessage) error {
	return r.db.WithContext(ctx).Create(message).Error
}

func (r *ChatMessageRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ChatMessage, error) {
	var message model.ChatMessage
	err := r.db.WithContext(ctx).
		Preload("User").
		First(&message, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &message, nil
}

func (r *ChatMessageRepository) GetBySession(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.ChatMessage, int64, error) {
	var messages []model.ChatMessage
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ChatMessage{}).
		Where("session_id = ? AND is_deleted = ?", sessionID, false)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := utils.ApplyPagination(query, page, pageSize).
		Preload("User").
		Order("created_at DESC").
		Find(&messages).Error

	return messages, total, err
}

func (r *ChatMessageRepository) GetBySessionWithReplies(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.ChatMessage, int64, error) {
	var messages []model.ChatMessage
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ChatMessage{}).
		Where("session_id = ? AND is_deleted = ? AND parent_id IS NULL", sessionID, false)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := utils.ApplyPagination(query, page, pageSize).
		Preload("User").
		Preload("Parent").
		Order("created_at DESC").
		Find(&messages).Error

	return messages, total, err
}

func (r *ChatMessageRepository) Update(ctx context.Context, message *model.ChatMessage) error {
	return r.db.WithContext(ctx).Save(message).Error
}

func (r *ChatMessageRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.ChatMessage{}, "id = ?", id).Error
}

func (r *ChatMessageRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.ChatMessage{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_deleted": true,
			"deleted_at": gorm.Expr("CURRENT_TIMESTAMP"),
			"deleted_by": deletedBy,
		}).Error
}

func (r *ChatMessageRepository) Pin(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.ChatMessage{}).
		Where("id = ?", id).
		Update("is_pinned", true).Error
}

func (r *ChatMessageRepository) UnPin(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.ChatMessage{}).
		Where("id = ?", id).
		Update("is_pinned", false).Error
}

func (r *ChatMessageRepository) CountBySession(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.ChatMessage{}).
		Where("session_id = ? AND is_deleted = ?", sessionID, false).
		Count(&count).Error
	return count, err
}
