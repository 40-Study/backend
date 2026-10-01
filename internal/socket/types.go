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
	// EventConversationBlockedChanged: DM 1-1 vừa bị khoá/mở khoá do chặn/bỏ chặn. Gửi tới kênh user:<id> của
	// CẢ HAI người với cùng một payload (ConversationBlockedChangedPayload), không nói ai chặn ai.
	EventConversationBlockedChanged = "conversation_blocked_changed"
)

// ConversationBlockedChangedPayload — payload của EventConversationBlockedChanged. IsBlocked đúng bằng cờ
// is_blocked của GET /conversations/:id (có chặn ở BẤT KỲ chiều nào): hai phía thấy giá trị giống hệt nhau.
type ConversationBlockedChangedPayload struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	IsBlocked      bool      `json:"is_blocked"`
}

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

// TypingGuard là phần TUỲ CHỌN của authorizer: quyết định có chuyển tiếp "đang gõ" của userID vào hội thoại
// không (DM 1-1 mà hai người chặn nhau thì không). Authorizer không cài đặt thì mọi "đang gõ" của người
// đã đăng ký kênh đều được chuyển tiếp như trước.
type TypingGuard interface {
	CanBroadcastTyping(userID uuid.UUID, conversationID uuid.UUID) (bool, error)
}

// typingAllowed — false thì BỎ IM LẶNG sự kiện gõ (không báo lỗi: người gõ không cần biết, ô nhập của họ đã bị
// khoá; và phía bên kia tuyệt đối không nhận gì). Lỗi hạ tầng hoặc id sai định dạng cũng bỏ (fail-closed): "đang
// gõ" chỉ là tín hiệu thoáng qua, bỏ nhầm một lần không hại gì, còn lọt nhầm là rò rỉ cho người đã chặn.
func typingAllowed(authorizer ChannelAuthorizer, userID uuid.UUID, conversationID string) bool {
	guard, ok := authorizer.(TypingGuard)
	if !ok {
		return true
	}
	convID, err := uuid.Parse(conversationID)
	if err != nil {
		return false
	}
	allowed, err := guard.CanBroadcastTyping(userID, convID)
	return err == nil && allowed
}
