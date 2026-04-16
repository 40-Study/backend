package socket

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Event types for WebSocket messages
const (
	EventNotification     = "notification"
	EventChatMessage      = "chat_message"
	EventUserOnline       = "user_online"
	EventUserOffline      = "user_offline"
	EventTyping           = "typing"
	EventCourseUpdate     = "course_update"
	EventEnrollmentUpdate = "enrollment_update"
	EventPaymentStatus    = "payment_status"
	EventAchievement      = "achievement"
	EventError            = "error"
	EventPing             = "ping"
	EventPong             = "pong"

	// Conversation messaging events
	EventConversationMessage = "conversation_message"
	EventMessageEdited       = "message_edited"
	EventMessageDeleted      = "message_deleted"
	EventMessageReaction     = "message_reaction"
	EventConversationTyping  = "conversation_typing"
)

// gửi đi
type Message struct {
	Event   string      `json:"event"`
	Payload interface{} `json:"payload,omitempty"`
}

// Nhận vào client -> server
type IncomingMessage struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// map 1:1 với model Notification trong model
type NotificationPayload struct {
	ID               uuid.UUID  `json:"id"`
	Title            string     `json:"title"`
	Content          string     `json:"content"`
	NotificationType string     `json:"notification_type"`
	ReferenceType    *string    `json:"reference_type,omitempty"`
	ReferenceID      *uuid.UUID `json:"reference_id,omitempty"`
	CreatedAt        string     `json:"created_at"`
}

// ChatMessagePayload represents a chat message payload
type ChatMessagePayload struct {
	RoomID    string    `json:"room_id"`
	SenderID  uuid.UUID `json:"sender_id"`
	Content   string    `json:"content"`
	Timestamp string    `json:"timestamp"`
}

// TypingPayload represents a typing indicator payload
type TypingPayload struct {
	RoomID   string    `json:"room_id"`
	UserID   uuid.UUID `json:"user_id"`
	UserName string    `json:"user_name"`
	IsTyping bool      `json:"is_typing"`
}

// UserStatusPayload represents user online/offline status
type UserStatusPayload struct {
	UserID   uuid.UUID `json:"user_id"`
	UserName string    `json:"user_name"`
	IsOnline bool      `json:"is_online"`
}

// ErrorPayload represents an error message
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ConversationTypingPayload represents typing in a conversation
type ConversationTypingPayload struct {
	ConversationID string    `json:"conversation_id"`
	UserID         uuid.UUID `json:"user_id"`
	UserName       string    `json:"user_name"`
	IsTyping       bool      `json:"is_typing"`
}

// SubscribePayload - client gửi để join/leave channel
type SubscribePayload struct {
	Channel string `json:"channel"`
}

// ChannelAuthorizer kiểm tra quyền subscribe channel
type ChannelAuthorizer interface {
	CanSubscribe(userID uuid.UUID, channel string) (bool, error)
}
