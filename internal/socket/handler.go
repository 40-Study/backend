package socket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type Handler struct {
	hub        *Hub
	authorizer ChannelAuthorizer
}

func NewHandler(hub *Hub, authorizer ChannelAuthorizer) *Handler {
	return &Handler{hub: hub, authorizer: authorizer}
}

// FiberClient wraps a websocket.Conn for use with Hub
type FiberClient struct {
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
	// hubClient là con trỏ DUY NHẤT hub biết tới (S6): Register, Unregister và đăng ký kênh đều phải dùng đúng
	// con trỏ này. Trước đây mỗi chỗ tạo một &Client mới nên Unregister không bao giờ tìm thấy client đã đăng ký,
	// kênh của hub không có Send để phát tin, và client đã đóng vẫn bị phát tới (panic send on closed channel).
	hubClient *Client
}

func newFiberClient(userID uuid.UUID, conn *websocket.Conn, hub *Hub, authorizer ChannelAuthorizer) *FiberClient {
	return &FiberClient{
		ID:         uuid.New(),
		UserID:     userID,
		Conn:       conn,
		Hub:        hub,
		Send:       make(chan []byte, 256),
		authorizer: authorizer,
		channels:   make(map[string]bool),
	}
}

// HandleWebSocket returns the Fiber websocket handler
func (h *Handler) HandleWebSocket(c *fiber.Ctx) error {
	// Check if it's a websocket upgrade request
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}

	// Get user_id from auth middleware (stored before upgrade)
	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized: missing user_id",
		})
	}

	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user_id",
		})
	}

	// Store userID for use in websocket handler
	c.Locals("ws_user_id", userID)

	// Use the websocket handler
	return websocket.New(func(conn *websocket.Conn) {
		// Get userID that was stored before upgrade
		wsUserID := conn.Locals("ws_user_id").(uuid.UUID)

		client := newFiberClient(wsUserID, conn, h.hub, h.authorizer)
		client.hubClient = &Client{
			ID:         client.ID,
			UserID:     client.UserID,
			Hub:        h.hub,
			Send:       client.Send,
			authorizer: client.authorizer,
			channels:   client.channels,
		}

		// Register client with hub
		h.hub.Register <- client.hubClient

		log.Printf("[WS] Client connected: user=%s, client=%s", client.UserID, client.ID)

		// Auto-subscribe to personal notification channel
		personalChannel := "user:" + client.UserID.String()
		client.channels[personalChannel] = true
		h.hub.SubscribeToChannel(client.hubClient, personalChannel)

		// Start writer goroutine
		go client.writePump()

		// Read pump (blocking)
		client.readPump()

		log.Printf("[WS] Client disconnected: user=%s, client=%s", client.UserID, client.ID)
	}, websocket.Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	})(c)
}

func (c *FiberClient) readPump() {
	defer func() {
		c.Hub.Unregister <- c.hubClient
		c.close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msg, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS] Read error: %v", err)
			}
			break
		}

		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		c.handleMessage(msg)
	}
}

func (c *FiberClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *FiberClient) handleMessage(data []byte) {
	var msg IncomingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		c.sendError("invalid_json", "Invalid message format")
		return
	}

	switch msg.Event {
	case EventPing:
		c.sendMessage(Message{Event: EventPong})

	case "subscribe":
		var payload SubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			c.subscribe(payload.Channel)
		}

	case "unsubscribe":
		var payload SubscribePayload
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			c.unsubscribe(payload.Channel)
		}

	case EventConversationTyping:
		// Forward typing indicator to conversation channel
		var payload ConversationTypingPayload
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			payload.UserID = c.UserID
			channelName := "conversation:" + payload.ConversationID
			// S6: chỉ chuyển tiếp "đang gõ" vào kênh mà chính client này đã đăng ký được (đã qua authorizer);
			// trước đây gửi được vào hội thoại bất kỳ chỉ cần biết id.
			c.channelMu.RLock()
			subscribed := c.channels[channelName]
			c.channelMu.RUnlock()
			// Kiểm lại quyền ngay lúc gõ: bị kick/ban sau khi đăng ký thì c.channels vẫn còn kênh cũ.
			if subscribed && c.authorizer != nil {
				if ok, err := c.authorizer.CanSubscribe(c.UserID, channelName); err != nil || !ok {
					subscribed = false
				}
			}
			if !subscribed {
				c.sendError("typing_denied", "You are not subscribed to this conversation")
				return
			}
			c.Hub.SendToChannel(channelName, Message{
				Event:   EventConversationTyping,
				Payload: payload,
			})
		}

	default:
		log.Printf("[WS] Unknown event: %s", msg.Event)
	}
}

func (c *FiberClient) sendMessage(msg Message) error {
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

func (c *FiberClient) sendError(code, message string) {
	c.sendMessage(Message{
		Event: EventError,
		Payload: ErrorPayload{
			Code:    code,
			Message: message,
		},
	})
}

func (c *FiberClient) subscribe(channel string) {
	if c.authorizer != nil {
		allowed, err := c.authorizer.CanSubscribe(c.UserID, channel)
		if err != nil || !allowed {
			c.sendError("subscribe_denied", "You don't have permission to subscribe to this channel")
			return
		}
	}

	c.channelMu.Lock()
	c.channels[channel] = true
	c.channelMu.Unlock()

	c.Hub.SubscribeToChannel(c.hubClient, channel)

	// Kiểm lại quyền SAU khi đăng ký (xem Client.Subscribe): đóng khe giữa kiểm quyền và đăng ký khi người dùng
	// bị gỡ khỏi nhóm đúng lúc đó.
	if c.authorizer != nil {
		if allowed, err := c.authorizer.CanSubscribe(c.UserID, channel); err != nil || !allowed {
			c.unsubscribe(channel)
			c.sendError("subscribe_denied", "You don't have permission to subscribe to this channel")
		}
	}
}

func (c *FiberClient) unsubscribe(channel string) {
	c.channelMu.Lock()
	delete(c.channels, channel)
	c.channelMu.Unlock()

	c.Hub.UnsubscribeFromChannel(c.hubClient, channel)
}

func (c *FiberClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.Send)
	c.Conn.Close()
}
