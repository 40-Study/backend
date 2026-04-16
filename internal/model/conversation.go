package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ============================================================================
// CONVERSATION ENUMS
// ============================================================================

type ConversationType string

const (
	ConversationTypeDirect ConversationType = "DIRECT"
	ConversationTypeGroup  ConversationType = "GROUP"
)

type MessageType string

const (
	MessageTypeText         MessageType = "TEXT"
	MessageTypeImage        MessageType = "IMAGE"
	MessageTypeFile         MessageType = "FILE"
	MessageTypeVideo        MessageType = "VIDEO"
	MessageTypeAudio        MessageType = "AUDIO"
	MessageTypeSticker      MessageType = "STICKER"
	MessageTypeSystem       MessageType = "SYSTEM"
	MessageTypeCoinGift     MessageType = "COIN_GIFT"
	MessageTypeSharedLesson MessageType = "SHARED_LESSON"
	MessageTypeSharedCourse MessageType = "SHARED_COURSE"
	MessageTypePoll         MessageType = "POLL"
)

type MessageStatus string

const (
	MessageStatusSending   MessageStatus = "SENDING"
	MessageStatusSent      MessageStatus = "SENT"
	MessageStatusDelivered MessageStatus = "DELIVERED"
	MessageStatusFailed    MessageStatus = "FAILED"
)

// ============================================================================
// CONVERSATION
// ============================================================================

// Conversation represents a chat conversation (direct or group)
type Conversation struct {
	BaseModel
	Type          ConversationType `gorm:"type:varchar(20);not null" json:"type"`
	GroupID       *uuid.UUID       `gorm:"type:uuid;uniqueIndex" json:"group_id,omitempty"`
	Name          *string          `gorm:"type:varchar(200)" json:"name,omitempty"`
	LastMessageID *uuid.UUID       `gorm:"type:uuid" json:"last_message_id,omitempty"`
	LastMessageAt *time.Time       `json:"last_message_at,omitempty"`
	MessageCount  int64            `gorm:"default:0" json:"message_count"`

	// Relationships
	Group        *Group                    `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	Participants []ConversationParticipant `gorm:"foreignKey:ConversationID" json:"participants,omitempty"`
	Messages     []Message                 `gorm:"foreignKey:ConversationID" json:"-"`
	LastMessage  *Message                  `gorm:"foreignKey:LastMessageID;constraint:-" json:"last_message,omitempty"`
}

func (Conversation) TableName() string {
	return "conversations"
}

// ============================================================================
// CONVERSATION PARTICIPANT
// ============================================================================

// ConversationParticipant represents a user participating in a conversation
type ConversationParticipant struct {
	BaseModel
	ConversationID    uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_conv_participant_unique,priority:1" json:"conversation_id"`
	UserID            uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_conv_participant_unique,priority:2" json:"user_id"`
	LastReadMessageID *uuid.UUID `gorm:"type:uuid" json:"last_read_message_id,omitempty"`
	LastReadAt        *time.Time `json:"last_read_at,omitempty"`
	UnreadCount       int        `gorm:"default:0" json:"unread_count"`
	IsMuted           bool       `gorm:"default:false" json:"is_muted"`
	MutedUntil        *time.Time `json:"muted_until,omitempty"`
	IsPinned          bool       `gorm:"default:false" json:"is_pinned"`
	JoinedAt          time.Time  `gorm:"default:now()" json:"joined_at"`
	LeftAt            *time.Time `json:"left_at,omitempty"`

	// Relationships
	Conversation    Conversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE" json:"-"`
	User            User         `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	LastReadMessage *Message     `gorm:"foreignKey:LastReadMessageID;constraint:-" json:"last_read_message,omitempty"`
}

func (ConversationParticipant) TableName() string {
	return "conversation_participants"
}

// ============================================================================
// MESSAGE
// ============================================================================

// Message represents a chat message
type Message struct {
	BaseModel
	ConversationID uuid.UUID      `gorm:"type:uuid;not null;index" json:"conversation_id"`
	SenderID       *uuid.UUID     `gorm:"type:uuid;index" json:"sender_id,omitempty"`
	Type           MessageType    `gorm:"type:varchar(20);default:'TEXT'" json:"type"`
	Content        *string        `gorm:"type:text" json:"content,omitempty"`
	Metadata       datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	ReplyToID      *uuid.UUID     `gorm:"type:uuid;index" json:"reply_to_id,omitempty"`
	Status         MessageStatus  `gorm:"type:varchar(20);default:'SENT'" json:"status"`
	IsEdited       bool           `gorm:"default:false" json:"is_edited"`
	EditedAt       *time.Time     `json:"edited_at,omitempty"`
	IsPinned       bool           `gorm:"default:false" json:"is_pinned"`
	PinnedBy       *uuid.UUID     `gorm:"type:uuid" json:"pinned_by,omitempty"`
	PinnedAt       *time.Time     `json:"pinned_at,omitempty"`

	// Relationships
	Conversation Conversation         `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE" json:"-"`
	Sender       *User                `gorm:"foreignKey:SenderID" json:"sender,omitempty"`
	ReplyTo      *Message             `gorm:"foreignKey:ReplyToID" json:"reply_to,omitempty"`
	Attachments  []MessageAttachment  `gorm:"foreignKey:MessageID" json:"attachments,omitempty"`
	Reactions    []MessageReaction    `gorm:"foreignKey:MessageID" json:"reactions,omitempty"`
	Mentions     []MessageMention     `gorm:"foreignKey:MessageID" json:"mentions,omitempty"`
}

func (Message) TableName() string {
	return "messages"
}

// ============================================================================
// MESSAGE REACTION
// ============================================================================

// MessageReaction represents an emoji reaction to a message
type MessageReaction struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	MessageID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_message_reaction_unique,priority:1" json:"message_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_message_reaction_unique,priority:2" json:"user_id"`
	Emoji     string    `gorm:"type:varchar(20);not null;uniqueIndex:idx_message_reaction_unique,priority:3" json:"emoji"`

	// Relationships
	Message Message `gorm:"foreignKey:MessageID;constraint:OnDelete:CASCADE" json:"-"`
	User    User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (MessageReaction) TableName() string {
	return "message_reactions"
}

// ============================================================================
// MESSAGE ATTACHMENT
// ============================================================================

// MessageAttachment represents a file attached to a message
type MessageAttachment struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	MessageID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"message_id"`
	FileName     string         `gorm:"type:varchar(255);not null" json:"file_name"`
	FileURL      string         `gorm:"type:text;not null" json:"file_url"`
	FileSize     *int64         `json:"file_size,omitempty"`
	MimeType     *string        `gorm:"type:varchar(100)" json:"mime_type,omitempty"`
	ThumbnailURL *string        `gorm:"type:text" json:"thumbnail_url,omitempty"`
	Metadata     datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"metadata"`

	// Relationships
	Message Message `gorm:"foreignKey:MessageID;constraint:OnDelete:CASCADE" json:"-"`
}

func (MessageAttachment) TableName() string {
	return "message_attachments"
}

// ============================================================================
// MESSAGE MENTION
// ============================================================================

// MessageMention represents a @mention in a message
type MessageMention struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	MessageID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_message_mention_unique,priority:1" json:"message_id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_message_mention_unique,priority:2" json:"user_id"`
	StartOffset *int      `json:"start_offset,omitempty"`
	EndOffset   *int      `json:"end_offset,omitempty"`

	// Relationships
	Message Message `gorm:"foreignKey:MessageID;constraint:OnDelete:CASCADE" json:"-"`
	User    User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (MessageMention) TableName() string {
	return "message_mentions"
}
