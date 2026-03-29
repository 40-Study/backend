package socket

import (
	"time"

	"github.com/google/uuid"
)

// Notifier provides methods for sending notifications via WebSocket
type Notifier struct {
	hub *Hub
}

// NewNotifier creates a new Notifier instance
func NewNotifier(hub *Hub) *Notifier {
	return &Notifier{hub: hub}
}

// SendNotification sends a notification to a specific user
func (n *Notifier) SendNotification(userID uuid.UUID, notification NotificationPayload) {
	n.hub.SendToUser(userID, Message{
		Event:   EventNotification,
		Payload: notification,
	})
}

// SendNotificationToMany sends a notification to multiple users
func (n *Notifier) SendNotificationToMany(userIDs []uuid.UUID, notification NotificationPayload) {
	msg := Message{
		Event:   EventNotification,
		Payload: notification,
	}
	n.hub.SendToUsers(userIDs, msg)
}

// BroadcastNotification sends a notification to all connected users
func (n *Notifier) BroadcastNotification(notification NotificationPayload) {
	n.hub.BroadcastAll(Message{
		Event:   EventNotification,
		Payload: notification,
	})
}

// SendChatMessage sends a chat message to a room
func (n *Notifier) SendChatMessage(roomID string, message ChatMessagePayload) {
	n.hub.SendToChannel(roomID, Message{
		Event:   EventChatMessage,
		Payload: message,
	})
}

// SendTypingIndicator sends a typing indicator to a room
func (n *Notifier) SendTypingIndicator(roomID string, typing TypingPayload) {
	n.hub.SendToChannel(roomID, Message{
		Event:   EventTyping,
		Payload: typing,
	})
}

// SendCourseUpdate sends a course update notification
func (n *Notifier) SendCourseUpdate(userIDs []uuid.UUID, payload interface{}) {
	msg := Message{
		Event:   EventCourseUpdate,
		Payload: payload,
	}
	n.hub.SendToUsers(userIDs, msg)
}

// SendEnrollmentUpdate sends an enrollment update
func (n *Notifier) SendEnrollmentUpdate(userID uuid.UUID, payload interface{}) {
	n.hub.SendToUser(userID, Message{
		Event:   EventEnrollmentUpdate,
		Payload: payload,
	})
}

// SendPaymentStatus sends a payment status update
func (n *Notifier) SendPaymentStatus(userID uuid.UUID, payload interface{}) {
	n.hub.SendToUser(userID, Message{
		Event:   EventPaymentStatus,
		Payload: payload,
	})
}

// SendAchievement sends an achievement notification
func (n *Notifier) SendAchievement(userID uuid.UUID, payload interface{}) {
	n.hub.SendToUser(userID, Message{
		Event:   EventAchievement,
		Payload: payload,
	})
}

// SendToChannel sends a custom message to a channel
func (n *Notifier) SendToChannel(channel string, event string, payload interface{}) {
	n.hub.SendToChannel(channel, Message{
		Event:   event,
		Payload: payload,
	})
}

// SendToUser sends a custom message to a user
func (n *Notifier) SendToUser(userID uuid.UUID, event string, payload interface{}) {
	n.hub.SendToUser(userID, Message{
		Event:   event,
		Payload: payload,
	})
}

// BroadcastAll sends a custom message to all users
func (n *Notifier) BroadcastAll(event string, payload interface{}) {
	n.hub.BroadcastAll(Message{
		Event:   event,
		Payload: payload,
	})
}

// IsUserOnline checks if a user is currently connected
func (n *Notifier) IsUserOnline(userID uuid.UUID) bool {
	return n.hub.IsUserOnline(userID)
}

// GetOnlineUsers returns all online user IDs
func (n *Notifier) GetOnlineUsers() []uuid.UUID {
	return n.hub.GetOnlineUsers()
}

// GetStats returns hub statistics
func (n *Notifier) GetStats() HubStats {
	return n.hub.Stats()
}

// NotifyNewNotification is a helper to create and send a notification
func (n *Notifier) NotifyNewNotification(
	userID uuid.UUID,
	id uuid.UUID,
	title string,
	content string,
	notificationType string,
	referenceType *string,
	referenceID *uuid.UUID,
) {
	n.SendNotification(userID, NotificationPayload{
		ID:               id,
		Title:            title,
		Content:          content,
		NotificationType: notificationType,
		ReferenceType:    referenceType,
		ReferenceID:      referenceID,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	})
}
