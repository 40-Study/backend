package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// CONVERSATION DTOs
// ============================================================================

type CreateDirectConversationRequest struct {
	UserID uuid.UUID `json:"user_id" validate:"required"`
}

type ConversationBrief struct {
	ID uuid.UUID `json:"id"`
}

type ConversationResponse struct {
	ID            uuid.UUID                `json:"id"`
	Type          string                   `json:"type"`
	Name          *string                  `json:"name,omitempty"`
	GroupID       *uuid.UUID               `json:"group_id,omitempty"`
	LastMessage   *MessageResponse         `json:"last_message,omitempty"`
	LastMessageAt *time.Time               `json:"last_message_at,omitempty"`
	MessageCount  int64                    `json:"message_count"`
	UnreadCount   int                      `json:"unread_count"`
	IsMuted       bool                     `json:"is_muted"`
	IsPinned      bool                     `json:"is_pinned"`
	Participants  []ParticipantResponse    `json:"participants,omitempty"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}

type ConversationListResponse struct {
	Conversations []ConversationResponse `json:"conversations"`
	TotalCount    int64                  `json:"total_count"`
	Page          int                    `json:"page"`
	Limit         int                    `json:"limit"`
}

type ParticipantResponse struct {
	UserID    uuid.UUID  `json:"user_id"`
	UserName  string     `json:"user_name"`
	Email     string     `json:"email"`
	AvatarURL *string    `json:"avatar_url,omitempty"`
	IsOnline  bool       `json:"is_online"`
	JoinedAt  time.Time  `json:"joined_at"`
	LastReadAt *time.Time `json:"last_read_at,omitempty"`
}

// ============================================================================
// MESSAGE DTOs
// ============================================================================

type SendMessageRequest struct {
	Content   *string     `json:"content,omitempty" validate:"required_without=Attachments"`
	Type      string      `json:"type" validate:"omitempty,oneof=TEXT IMAGE FILE VIDEO AUDIO STICKER COIN_GIFT SHARED_LESSON SHARED_COURSE POLL"`
	ReplyToID *uuid.UUID  `json:"reply_to_id,omitempty"`
	Metadata  interface{} `json:"metadata,omitempty"`
}

type EditMessageRequest struct {
	Content string `json:"content" validate:"required,min=1"`
}

type MessageResponse struct {
	ID             uuid.UUID                  `json:"id"`
	ConversationID uuid.UUID                  `json:"conversation_id"`
	SenderID       *uuid.UUID                 `json:"sender_id,omitempty"`
	SenderName     string                     `json:"sender_name"`
	SenderAvatar   *string                    `json:"sender_avatar,omitempty"`
	Type           string                     `json:"type"`
	Content        *string                    `json:"content,omitempty"`
	Metadata       interface{}                `json:"metadata,omitempty"`
	ReplyTo        *MessageReplyResponse      `json:"reply_to,omitempty"`
	Status         string                     `json:"status"`
	IsEdited       bool                       `json:"is_edited"`
	EditedAt       *time.Time                 `json:"edited_at,omitempty"`
	IsPinned       bool                       `json:"is_pinned"`
	Reactions      []MessageReactionResponse  `json:"reactions,omitempty"`
	Attachments    []MessageAttachmentResponse `json:"attachments,omitempty"`
	CreatedAt      time.Time                  `json:"created_at"`
}

type MessageReplyResponse struct {
	ID         uuid.UUID  `json:"id"`
	SenderID   *uuid.UUID `json:"sender_id,omitempty"`
	SenderName string     `json:"sender_name"`
	Content    *string    `json:"content,omitempty"`
	Type       string     `json:"type"`
}

type MessageListResponse struct {
	Messages   []MessageResponse `json:"messages"`
	TotalCount int64             `json:"total_count"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
}

type MessageReactionResponse struct {
	Emoji string      `json:"emoji"`
	Count int         `json:"count"`
	Users []uuid.UUID `json:"users"`
}

type MessageAttachmentResponse struct {
	ID           uuid.UUID `json:"id"`
	FileName     string    `json:"file_name"`
	FileURL      string    `json:"file_url"`
	FileSize     *int64    `json:"file_size,omitempty"`
	MimeType     *string   `json:"mime_type,omitempty"`
	ThumbnailURL *string   `json:"thumbnail_url,omitempty"`
}

type AddReactionRequest struct {
	Emoji string `json:"emoji" validate:"required,min=1,max=20"`
}

type UnreadCountResponse struct {
	TotalUnread    int                        `json:"total_unread"`
	Conversations  []ConversationUnreadCount  `json:"conversations"`
}

type ConversationUnreadCount struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	UnreadCount    int       `json:"unread_count"`
}

type SearchMessagesRequest struct {
	Query          string     `json:"query" validate:"required,min=1"`
	ConversationID *uuid.UUID `json:"conversation_id,omitempty"`
}
