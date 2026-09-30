package socket

// Lane S6: hub phải tiếp nhận nhiều kết nối liên tiếp. Trước đây registerClient giữ clientsMu.Lock rồi gọi
// BroadcastAll (RLock cùng mutex) nên goroutine Run tự khoá chết ở lần đăng ký đầu tiên và mọi kết nối sau treo.

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestS6_Hub_DangKyLienTiepKhongKhoaChet(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	register := func() *Client {
		c := &Client{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 8), channels: map[string]bool{}}
		done := make(chan struct{})
		go func() { hub.Register <- c; close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Register bị treo: goroutine Run đã khoá chết")
		}
		return c
	}
	for i := 0; i < 3; i++ {
		register()
	}
	// Register vô đệm chỉ báo "đã nhận", nên đợi hub xử lý xong rồi mới đếm.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && hub.Stats().OnlineUsers != 3 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := hub.Stats().OnlineUsers; got != 3 {
		t.Fatalf("OnlineUsers = %d, muốn 3", got)
	}
}
// Client đã đóng (Send bị close) mà hub còn giữ thì phát tin không được làm sập tiến trình.
func TestS6_Hub_PhatToiClientDaDongKhongPanic(t *testing.T) {
	hub := NewHub()
	c := &Client{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 1), channels: map[string]bool{}}
	hub.clients[c.UserID] = map[*Client]bool{c: true}
	close(c.Send)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("phát tới client đã đóng bị panic: %v", r)
		}
	}()
	hub.BroadcastAll(Message{Event: "x"})
	hub.SendToUser(c.UserID, Message{Event: "x"})
	if err := c.SendMessage(Message{Event: "x"}); err == nil {
		t.Fatal("SendMessage trên kênh đã đóng phải trả lỗi")
	}
}

// Unregister phải gỡ đúng client đã Register (cùng con trỏ) khỏi hub.
func TestS6_Hub_UnregisterGoDungClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	c := &Client{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 8), channels: map[string]bool{}}
	hub.Register <- c
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !hub.IsUserOnline(c.UserID) {
		time.Sleep(10 * time.Millisecond)
	}
	if !hub.IsUserOnline(c.UserID) {
		t.Fatal("client chưa online sau Register")
	}
	hub.Unregister <- c
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && hub.IsUserOnline(c.UserID) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.IsUserOnline(c.UserID) {
		t.Fatal("client vẫn online sau Unregister")
	}
}