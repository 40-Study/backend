package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type ConversationRepositoryInterface interface {
	Create(ctx context.Context, conv *model.Conversation) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Conversation, error)
	GetDirectBetweenUsers(ctx context.Context, userID1, userID2 uuid.UUID) (*model.Conversation, error)
	GetByGroupID(ctx context.Context, groupID uuid.UUID) (*model.Conversation, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Conversation, int64, error)
	UpdateLastMessage(ctx context.Context, convID, messageID uuid.UUID) error
	IncrementMessageCount(ctx context.Context, convID uuid.UUID) error
}

type ConversationParticipantRepositoryInterface interface {
	Create(ctx context.Context, p *model.ConversationParticipant) error
	CreateBatch(ctx context.Context, participants []model.ConversationParticipant) error
	GetByConvAndUser(ctx context.Context, convID, userID uuid.UUID) (*model.ConversationParticipant, error)
	ListByConversationID(ctx context.Context, convID uuid.UUID) ([]model.ConversationParticipant, error)
	MarkAsRead(ctx context.Context, convID, userID, messageID uuid.UUID) error
	UpdateMuted(ctx context.Context, convID, userID uuid.UUID, muted bool) error
	UpdatePinned(ctx context.Context, convID, userID uuid.UUID, pinned bool) error
	IncrementUnreadExcept(ctx context.Context, convID, senderID uuid.UUID) error
	GetTotalUnread(ctx context.Context, userID uuid.UUID) (int, error)
	GetUnreadCounts(ctx context.Context, userID uuid.UUID) ([]struct {
		ConversationID uuid.UUID
		UnreadCount    int
	}, error)
}

// ============================================================================
// CONVERSATION REPOSITORY
// ============================================================================

type ConversationRepository struct {
	db *gorm.DB
}

func NewConversationRepository(db *gorm.DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

func (r *ConversationRepository) Create(ctx context.Context, conv *model.Conversation) error {
	return r.db.WithContext(ctx).Create(conv).Error
}

func (r *ConversationRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).
		Preload("Participants.User").
		Preload("LastMessage.Sender").
		First(&conv, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &conv, nil
}

func (r *ConversationRepository) GetDirectBetweenUsers(ctx context.Context, userID1, userID2 uuid.UUID) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).
		Where("type = ? AND id IN (SELECT conversation_id FROM conversation_participants WHERE user_id = ? INTERSECT SELECT conversation_id FROM conversation_participants WHERE user_id = ?)",
			model.ConversationTypeDirect, userID1, userID2).
		Preload("Participants.User").
		First(&conv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &conv, nil
}

func (r *ConversationRepository) GetByGroupID(ctx context.Context, groupID uuid.UUID) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		First(&conv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &conv, nil
}

func (r *ConversationRepository) ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Conversation, int64, error) {
	var conversations []model.Conversation
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Conversation{}).
		Joins("JOIN conversation_participants ON conversation_participants.conversation_id = conversations.id").
		Where("conversation_participants.user_id = ? AND conversation_participants.left_at IS NULL", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = utils.ApplyPagination(query, page, pageSize)
	err := query.
		Preload("Participants.User").
		Preload("LastMessage.Sender").
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").
		Find(&conversations).Error
	return conversations, total, err
}

func (r *ConversationRepository) UpdateLastMessage(ctx context.Context, convID, messageID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.Conversation{}).
		Where("id = ?", convID).
		Updates(map[string]interface{}{
			"last_message_id": messageID,
			"last_message_at": gorm.Expr("NOW()"),
		}).Error
}

func (r *ConversationRepository) IncrementMessageCount(ctx context.Context, convID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.Conversation{}).
		Where("id = ?", convID).
		UpdateColumn("message_count", gorm.Expr("message_count + 1")).Error
}

// ============================================================================
// CONVERSATION PARTICIPANT REPOSITORY
// ============================================================================

type ConversationParticipantRepository struct {
	db *gorm.DB
}

func NewConversationParticipantRepository(db *gorm.DB) *ConversationParticipantRepository {
	return &ConversationParticipantRepository{db: db}
}

func (r *ConversationParticipantRepository) Create(ctx context.Context, p *model.ConversationParticipant) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *ConversationParticipantRepository) CreateBatch(ctx context.Context, participants []model.ConversationParticipant) error {
	return r.db.WithContext(ctx).Create(&participants).Error
}

func (r *ConversationParticipantRepository) GetByConvAndUser(ctx context.Context, convID, userID uuid.UUID) (*model.ConversationParticipant, error) {
	var p model.ConversationParticipant
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ? AND left_at IS NULL", convID, userID).
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *ConversationParticipantRepository) ListByConversationID(ctx context.Context, convID uuid.UUID) ([]model.ConversationParticipant, error) {
	var participants []model.ConversationParticipant
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND left_at IS NULL", convID).
		Preload("User").
		Find(&participants).Error
	return participants, err
}

func (r *ConversationParticipantRepository) MarkAsRead(ctx context.Context, convID, userID, messageID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", convID, userID).
		Updates(map[string]interface{}{
			"last_read_message_id": messageID,
			"last_read_at":         gorm.Expr("NOW()"),
			"unread_count":         0,
		}).Error
}

func (r *ConversationParticipantRepository) UpdateMuted(ctx context.Context, convID, userID uuid.UUID, muted bool) error {
	return r.db.WithContext(ctx).Model(&model.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", convID, userID).
		Update("is_muted", muted).Error
}

func (r *ConversationParticipantRepository) UpdatePinned(ctx context.Context, convID, userID uuid.UUID, pinned bool) error {
	return r.db.WithContext(ctx).Model(&model.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", convID, userID).
		Update("is_pinned", pinned).Error
}

func (r *ConversationParticipantRepository) IncrementUnreadExcept(ctx context.Context, convID, senderID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id != ? AND left_at IS NULL", convID, senderID).
		UpdateColumn("unread_count", gorm.Expr("unread_count + 1")).Error
}

func (r *ConversationParticipantRepository) GetTotalUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	var total int
	err := r.db.WithContext(ctx).Model(&model.ConversationParticipant{}).
		Select("COALESCE(SUM(unread_count), 0)").
		Where("user_id = ? AND left_at IS NULL", userID).
		Scan(&total).Error
	return total, err
}

func (r *ConversationParticipantRepository) GetUnreadCounts(ctx context.Context, userID uuid.UUID) ([]struct {
	ConversationID uuid.UUID
	UnreadCount    int
}, error) {
	var results []struct {
		ConversationID uuid.UUID
		UnreadCount    int
	}
	err := r.db.WithContext(ctx).Model(&model.ConversationParticipant{}).
		Select("conversation_id, unread_count").
		Where("user_id = ? AND left_at IS NULL AND unread_count > 0", userID).
		Scan(&results).Error
	return results, err
}
