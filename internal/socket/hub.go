package socket

import (
	"log"
	"sync"

	"github.com/google/uuid"
)

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients   map[uuid.UUID]map[*Client]bool
	clientsMu sync.RWMutex

	// Channel subscriptions
	channels   map[string]map[*Client]bool
	channelsMu sync.RWMutex

	// Register requests from clients
	Register chan *Client

	// Unregister requests from clients
	Unregister chan *Client

	// Message handlers
	messageHandlers   map[string]MessageHandler
	messageHandlersMu sync.RWMutex
}

// MessageHandler is a function that handles incoming messages
type MessageHandler func(client *Client, payload []byte)

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		clients:         make(map[uuid.UUID]map[*Client]bool),
		channels:        make(map[string]map[*Client]bool),
		Register:        make(chan *Client),
		Unregister:      make(chan *Client),
		messageHandlers: make(map[string]MessageHandler),
	}
}

// Run starts the hub's main event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.registerClient(client)

		case client := <-h.Unregister:
			h.unregisterClient(client)
		}
	}
}

// registerClient adds a client to the hub
func (h *Hub) registerClient(client *Client) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	if _, ok := h.clients[client.UserID]; !ok {
		h.clients[client.UserID] = make(map[*Client]bool)
	}
	h.clients[client.UserID][client] = true

	log.Printf("Client registered: user=%s, client=%s, total_connections=%d",
		client.UserID, client.ID, h.countConnections())

	// Broadcast user online status
	h.broadcastUserStatus(client.UserID, true)
}

// unregisterClient removes a client from the hub
func (h *Hub) unregisterClient(client *Client) {
	h.clientsMu.Lock()

	if clients, ok := h.clients[client.UserID]; ok {
		if _, ok := clients[client]; ok {
			delete(clients, client)
			if len(clients) == 0 {
				delete(h.clients, client.UserID)
				// User is now offline (no more connections)
				h.clientsMu.Unlock()
				h.broadcastUserStatus(client.UserID, false)
				h.clientsMu.Lock()
			}
		}
	}

	h.clientsMu.Unlock()

	// Remove from all channels
	h.channelsMu.Lock()
	for channel := range h.channels {
		delete(h.channels[channel], client)
		if len(h.channels[channel]) == 0 {
			delete(h.channels, channel)
		}
	}
	h.channelsMu.Unlock()

	log.Printf("Client unregistered: user=%s, client=%s", client.UserID, client.ID)
}

// countConnections returns total number of connections (internal use)
func (h *Hub) countConnections() int {
	count := 0
	for _, clients := range h.clients {
		count += len(clients)
	}
	return count
}

// broadcastUserStatus broadcasts user online/offline status
func (h *Hub) broadcastUserStatus(userID uuid.UUID, isOnline bool) {
	event := EventUserOffline
	if isOnline {
		event = EventUserOnline
	}

	msg := Message{
		Event: event,
		Payload: UserStatusPayload{
			UserID:   userID,
			IsOnline: isOnline,
		},
	}

	h.BroadcastAll(msg)
}

// SubscribeToChannel adds a client to a channel
func (h *Hub) SubscribeToChannel(client *Client, channel string) {
	h.channelsMu.Lock()
	defer h.channelsMu.Unlock()

	if _, ok := h.channels[channel]; !ok {
		h.channels[channel] = make(map[*Client]bool)
	}
	h.channels[channel][client] = true

	log.Printf("Client subscribed to channel: user=%s, channel=%s", client.UserID, channel)
}

// UnsubscribeFromChannel removes a client from a channel
func (h *Hub) UnsubscribeFromChannel(client *Client, channel string) {
	h.channelsMu.Lock()
	defer h.channelsMu.Unlock()

	if clients, ok := h.channels[channel]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.channels, channel)
		}
	}

	log.Printf("Client unsubscribed from channel: user=%s, channel=%s", client.UserID, channel)
}

// RegisterHandler registers a message handler for an event type
func (h *Hub) RegisterHandler(event string, handler MessageHandler) {
	h.messageHandlersMu.Lock()
	defer h.messageHandlersMu.Unlock()
	h.messageHandlers[event] = handler
}

// HandleClientMessage handles incoming messages from clients
func (h *Hub) HandleClientMessage(client *Client, msg IncomingMessage) {
	h.messageHandlersMu.RLock()
	handler, ok := h.messageHandlers[msg.Event]
	h.messageHandlersMu.RUnlock()

	if ok {
		handler(client, msg.Payload)
	}
}

// SendToUser sends a message to all connections of a specific user
func (h *Hub) SendToUser(userID uuid.UUID, msg Message) {
	h.clientsMu.RLock()
	clients, ok := h.clients[userID]
	h.clientsMu.RUnlock()

	if !ok {
		return
	}

	for client := range clients {
		_ = client.SendMessage(msg)
	}
}

// SendToUsers sends a message to multiple users
func (h *Hub) SendToUsers(userIDs []uuid.UUID, msg Message) {
	for _, userID := range userIDs {
		h.SendToUser(userID, msg)
	}
}

// SendToChannel sends a message to all clients in a channel
func (h *Hub) SendToChannel(channel string, msg Message) {
	h.channelsMu.RLock()
	clients, ok := h.channels[channel]
	h.channelsMu.RUnlock()

	if !ok {
		return
	}

	for client := range clients {
		_ = client.SendMessage(msg)
	}
}

// BroadcastAll sends a message to all connected clients
func (h *Hub) BroadcastAll(msg Message) {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	for _, clients := range h.clients {
		for client := range clients {
			_ = client.SendMessage(msg)
		}
	}
}

// IsUserOnline checks if a user has any active connections
func (h *Hub) IsUserOnline(userID uuid.UUID) bool {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	clients, ok := h.clients[userID]
	return ok && len(clients) > 0
}

// GetOnlineUsers returns a list of all online user IDs
func (h *Hub) GetOnlineUsers() []uuid.UUID {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	users := make([]uuid.UUID, 0, len(h.clients))
	for userID := range h.clients {
		users = append(users, userID)
	}
	return users
}

// GetUserConnectionCount returns the number of connections for a user
func (h *Hub) GetUserConnectionCount(userID uuid.UUID) int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	if clients, ok := h.clients[userID]; ok {
		return len(clients)
	}
	return 0
}

// GetChannelMembers returns all user IDs in a channel
func (h *Hub) GetChannelMembers(channel string) []uuid.UUID {
	h.channelsMu.RLock()
	defer h.channelsMu.RUnlock()

	clients, ok := h.channels[channel]
	if !ok {
		return nil
	}

	userSet := make(map[uuid.UUID]bool)
	for client := range clients {
		userSet[client.UserID] = true
	}

	users := make([]uuid.UUID, 0, len(userSet))
	for userID := range userSet {
		users = append(users, userID)
	}
	return users
}

// Stats returns hub statistics
func (h *Hub) Stats() HubStats {
	h.clientsMu.RLock()
	h.channelsMu.RLock()
	defer h.clientsMu.RUnlock()
	defer h.channelsMu.RUnlock()

	totalConnections := 0
	for _, clients := range h.clients {
		totalConnections += len(clients)
	}

	return HubStats{
		OnlineUsers:      len(h.clients),
		TotalConnections: totalConnections,
		ActiveChannels:   len(h.channels),
	}
}

// HubStats contains hub statistics
type HubStats struct {
	OnlineUsers      int `json:"online_users"`
	TotalConnections int `json:"total_connections"`
	ActiveChannels   int `json:"active_channels"`
}
