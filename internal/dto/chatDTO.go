package dto

import "github.com/google/uuid"

type SendChatMessageDTO struct {
	SessionID string `json:"session_id" validate:"required,uuid"`
	UserID    string `json:"user_id" validate:"required,uuid"`
	Message   string `json:"message" validate:"required,max=5000"`
}

type ChatMessageResponseDTO struct {
	ID        uuid.UUID  `json:"id"`
	SessionID uuid.UUID  `json:"session_id"`
	UserID    uuid.UUID  `json:"user_id"`
	UserName  string     `json:"user_name"`
	Message   string     `json:"message"`
	IsPinned  bool       `json:"is_pinned"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	CreatedAt string     `json:"created_at"`
}

type ChatMessageListDTO struct {
	Data     []ChatMessageResponseDTO `json:"data"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

type DeleteChatMessageDTO struct {
	MessageID string `json:"message_id" validate:"required,uuid"`
	DeletedBy string `json:"deleted_by" validate:"required,uuid"`
}

type PinChatMessageDTO struct {
	MessageID string `json:"message_id" validate:"required,uuid"`
	IsPinned  bool   `json:"is_pinned"`
}
