package socket

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/google/uuid"
)

var (
	ErrSendBufferFull   = errors.New("send buffer full")
	ErrConnectionClosed = errors.New("connection closed")
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
)

// Client represents a WebSocket client connection
type Client struct {
	ID         uuid.UUID          // unique per connection (1 user có thể nhiều Client)
	UserID     uuid.UUID          // từ JWT, identify user
	Conn       net.Conn           // raw tcp connection, không phải WebSocket connection vì đã hijack
	Hub        *Hub               // reference ngượck để gửi message đến hub
	Send       chan []byte        // channel để gửi message đến client
	authorizer ChannelAuthorizer  // kiểm tra quyền subscribe channel
	mu         sync.Mutex         // bảo vệ concurrent writes đến connection
	closed     bool               // đánh dấu connection đã đóng
	channels   map[string]bool    // kênh mà client đã subscribe
	channelMu  sync.RWMutex       // bảo vệ concurrent access đến channels map
}

// NewClient creates a new WebSocket client
func NewClient(userID uuid.UUID, conn net.Conn, hub *Hub, authorizer ChannelAuthorizer) *Client {
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

// ReadPump là hàm chạy trong goroutine riêng để liên tục đọc message từ client và xử lý chúng
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Close()
	}()

	// set deadline ban đầu - nếu không nhận được gì trong 60s thì đóng connection
	_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))

	// vòng lặp vô hạn để liên tục đọc message từ client
	for {
		msg, op, err := wsutil.ReadClientData(c.Conn)
		// msg : payload của message, có thể là text hoặc binary
		// op : loại message (text, binary, ping, pong, close)
		// err : lỗi nếu có (bao gồm timeout từ deadline)
		if err != nil {
			if !c.isClosed() {
				log.Printf("WebSocket read error for user %s: %v", c.UserID, err)
			}
			break
		}
		// close connection nếu client gửi frame close hoặc có lỗi đọc
		if op == ws.OpClose {
			break
		}
		// xử lý ping/pong để giữ kết nối sống
		if op == ws.OpPing {
			c.mu.Lock()
			_ = wsutil.WriteServerMessage(c.Conn, ws.OpPong, msg)
			c.mu.Unlock()
			// reset deadline khi nhận ping từ client
			_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
			continue
		}
		// nhận pong từ client (response cho ping của server) - reset deadline
		if op == ws.OpPong {
			_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
			continue
		}
		// chỉ xử lý message text, bỏ qua binary hoặc control frames khác
		if op == ws.OpText {
			// reset deadline khi nhận message hợp lệ
			_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
			c.handleMessage(msg)
		}
	}
}

// WritePump là hàm chạy trong goroutine riêng để liên tục gửi message đến client từ channel Send
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.writeControl(ws.OpClose, nil)
				return
			}

			c.mu.Lock()
			err := c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err != nil {
				c.mu.Unlock()
				return
			}

			err = wsutil.WriteServerText(c.Conn, message)
			c.mu.Unlock()

			if err != nil {
				log.Printf("WebSocket write error for user %s: %v", c.UserID, err)
				return
			}

		case <-ticker.C:
			if err := c.writeControl(ws.OpPing, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage xử lý message nhận được từ client
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
		// Forward to hub for broadcast or further processing
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
	_ = c.SendMessage(Message{
		Event: EventError,
		Payload: ErrorPayload{
			Code:    code,
			Message: message,
		},
	})
}

// Subscribe adds the client to a channel after checking authorization
func (c *Client) Subscribe(channel string) {
	if c.authorizer != nil {
		allowed, err := c.authorizer.CanSubscribe(c.UserID, channel)
		if err != nil {
			log.Printf("Channel auth error: user=%s, channel=%s, err=%v", c.UserID, channel, err)
			c.SendError("subscribe_error", "Failed to check channel permission")
			return
		}
		if !allowed {
			log.Printf("Channel auth denied: user=%s, channel=%s", c.UserID, channel)
			c.SendError("subscribe_denied", "You don't have permission to subscribe to this channel")
			return
		}
	}

	c.channelMu.Lock()
	c.channels[channel] = true
	c.channelMu.Unlock()
	c.Hub.SubscribeToChannel(c, channel)
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

// writeControl writes a control frame
func (c *Client) writeControl(op ws.OpCode, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrConnectionClosed
	}

	_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
	return wsutil.WriteServerMessage(c.Conn, op, data)
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
	_ = c.Conn.Close()
}

// isClosed checks if the connection is closed
func (c *Client) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
