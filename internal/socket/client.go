package socket

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"
)

var (
	ErrSendBufferFull   = errors.New("send buffer full")
	ErrConnectionClosed = errors.New("connection closed")
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

// Client represents a WebSocket client connection
type Client struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Conn       *websocket.Conn
	Hub        *Hub
	Send       chan []byte
	authorizer ChannelAuthorizer
	mu         sync.Mutex
	closed     bool
	channels   map[string]bool
	channelMu  sync.RWMutex
}

// NewClient creates a new WebSocket client
func NewClient(userID uuid.UUID, conn *websocket.Conn, hub *Hub, authorizer ChannelAuthorizer) *Client {
	return &Client{
		ID:         uuid.New(),
		UserID:     userID,
		Conn:       conn,
		Hub:        hub,
		Send:       make(chan []byte, 256),
		authorizer: authorizer,
		channels:   make(map[string]bool),
	}
}

// ReadPump reads messages from the WebSocket connection
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, msg, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS] Read error for user %s: %v", c.UserID, err)
			}
			break
		}

		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		c.handleMessage(msg)
	}
}

// WritePump writes messages to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("[WS] Write error for user %s: %v", c.UserID, err)
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages from client
func (c *Client) handleMessage(data []byte) {
	var msg IncomingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		c.SendError("invalid_json", "Invalid message format")
		return
	}

	switch msg.Event {
	case EventPing:
		c.SendMessage(Message{Event: EventPong})

	case "subscribe":
		var payload SubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			c.Subscribe(payload.Channel)
		}

	case "unsubscribe":
		var payload SubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			c.Unsubscribe(payload.Channel)
		}

	default:
		c.Hub.HandleClientMessage(c, msg)
	}
}

// SendMessage sends a message to the client
func (c *Client) SendMessage(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	select {
	case c.Send <- data:
		return nil
	default:
		return ErrSendBufferFull
	}
}

// SendError sends an error message to the client
func (c *Client) SendError(code, message string) {
	c.SendMessage(Message{
		Event: EventError,
		Payload: ErrorPayload{
			Code:    code,
			Message: message,
		},
	})
}

// Subscribe adds the client to a channel
func (c *Client) Subscribe(channel string) {
	if c.authorizer != nil {
		allowed, err := c.authorizer.CanSubscribe(c.UserID, channel)
		if err != nil || !allowed {
			c.SendError("subscribe_denied", "Permission denied for channel: "+channel)
			return
		}
	}

	c.channelMu.Lock()
	c.channels[channel] = true
	c.channelMu.Unlock()

	c.Hub.SubscribeToChannel(c, channel)
	log.Printf("[WS] User %s subscribed to channel: %s", c.UserID, channel)
}

// Unsubscribe removes the client from a channel
func (c *Client) Unsubscribe(channel string) {
	c.channelMu.Lock()
	delete(c.channels, channel)
	c.channelMu.Unlock()

	c.Hub.UnsubscribeFromChannel(c, channel)
}

// IsSubscribed checks if client is subscribed to a channel
func (c *Client) IsSubscribed(channel string) bool {
	c.channelMu.RLock()
	defer c.channelMu.RUnlock()
	return c.channels[channel]
}

// Close closes the client connection
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.Send)
	c.Conn.Close()
}

// IsClosed checks if the connection is closed
func (c *Client) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
