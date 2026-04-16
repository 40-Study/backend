package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type MessageRepositoryInterface interface {
	Create(ctx context.Context, msg *model.Message) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Message, error)
	Update(ctx context.Context, msg *model.Message) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByConversationID(ctx context.Context, convID uuid.UUID, page, pageSize int) ([]model.Message, int64, error)
	Search(ctx context.Context, userID uuid.UUID, keyword string, convID *uuid.UUID, page, pageSize int) ([]model.Message, int64, error)
	GetLastMessage(ctx context.Context, convID uuid.UUID) (*model.Message, error)
}

type MessageReactionRepositoryInterface interface {
	Create(ctx context.Context, reaction *model.MessageReaction) error
	Delete(ctx context.Context, messageID, userID uuid.UUID, emoji string) error
	GetByMessage(ctx context.Context, messageID uuid.UUID) ([]model.MessageReaction, error)
}

type MessageAttachmentRepositoryInterface interface {
	Create(ctx context.Context, attachment *model.MessageAttachment) error
	GetByMessageID(ctx context.Context, messageID uuid.UUID) ([]model.MessageAttachment, error)
}

// ============================================================================
// MESSAGE REPOSITORY
// ============================================================================

type MessageRepository struct {
	db *gorm.DB
}

func NewMessageRepository(db *gorm.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

func (r *MessageRepository) Create(ctx context.Context, msg *model.Message) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *MessageRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Message, error) {
	var msg model.Message
	err := r.db.WithContext(ctx).
		Preload("Sender").
		Preload("ReplyTo.Sender").
		Preload("Reactions").
		Preload("Attachments").
		First(&msg, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &msg, nil
}

func (r *MessageRepository) Update(ctx context.Context, msg *model.Message) error {
	return r.db.WithContext(ctx).Save(msg).Error
}

func (r *MessageRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Message{}, "id = ?", id).Error
}

func (r *MessageRepository) ListByConversationID(ctx context.Context, convID uuid.UUID, page, pageSize int) ([]model.Message, int64, error) {
	var messages []model.Message
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Message{}).
		Where("conversation_id = ?", convID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.
		Preload("Sender").
		Preload("ReplyTo.Sender").
		Preload("Reactions").
		Preload("Attachments").
		Order("created_at DESC").
		Find(&messages).Error
	return messages, total, err
}

func (r *MessageRepository) Search(ctx context.Context, userID uuid.UUID, keyword string, convID *uuid.UUID, page, pageSize int) ([]model.Message, int64, error) {
	var messages []model.Message
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Message{}).
		Joins("JOIN conversation_participants ON conversation_participants.conversation_id = messages.conversation_id").
		Where("conversation_participants.user_id = ? AND conversation_participants.left_at IS NULL", userID)

	if convID != nil {
		query = query.Where("messages.conversation_id = ?", *convID)
	}

	query = utils.ApplyKeywordSearch(query, keyword, "messages.content")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.
		Preload("Sender").
		Preload("Attachments").
		Order("messages.created_at DESC").
		Find(&messages).Error
	return messages, total, err
}

func (r *MessageRepository) GetLastMessage(ctx context.Context, convID uuid.UUID) (*model.Message, error) {
	var msg model.Message
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", convID).
		Order("created_at DESC").
		First(&msg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &msg, nil
}

// ============================================================================
// MESSAGE REACTION REPOSITORY
// ============================================================================

type MessageReactionRepository struct {
	db *gorm.DB
}

func NewMessageReactionRepository(db *gorm.DB) *MessageReactionRepository {
	return &MessageReactionRepository{db: db}
}

func (r *MessageReactionRepository) Create(ctx context.Context, reaction *model.MessageReaction) error {
	return r.db.WithContext(ctx).Create(reaction).Error
}

func (r *MessageReactionRepository) Delete(ctx context.Context, messageID, userID uuid.UUID, emoji string) error {
	return r.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).
		Delete(&model.MessageReaction{}).Error
}

func (r *MessageReactionRepository) GetByMessage(ctx context.Context, messageID uuid.UUID) ([]model.MessageReaction, error) {
	var reactions []model.MessageReaction
	err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Find(&reactions).Error
	return reactions, err
}

// ============================================================================
// MESSAGE ATTACHMENT REPOSITORY
// ============================================================================

type MessageAttachmentRepository struct {
	db *gorm.DB
}

func NewMessageAttachmentRepository(db *gorm.DB) *MessageAttachmentRepository {
	return &MessageAttachmentRepository{db: db}
}

func (r *MessageAttachmentRepository) Create(ctx context.Context, attachment *model.MessageAttachment) error {
	return r.db.WithContext(ctx).Create(attachment).Error
}

func (r *MessageAttachmentRepository) GetByMessageID(ctx context.Context, messageID uuid.UUID) ([]model.MessageAttachment, error) {
	var attachments []model.MessageAttachment
	err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Find(&attachments).Error
	return attachments, err
}
