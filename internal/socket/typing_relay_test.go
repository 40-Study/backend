package socket

// A-15: hai đường relay "đang gõ" (Client và FiberClient) đều phải gắn user_name do SERVER tra, không giữ giá trị
// client gửi. Bỏ dòng gán payload.UserName ở một trong hai đường thì test tương ứng ĐỎ. Tên chỉ tra một lần mỗi
// kết nối (typingNameCache).

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

type namingAuthorizer struct {
	stubAuthorizer
	name  string
	calls int
}

func (n *namingAuthorizer) TypingDisplayName(uuid.UUID) string { n.calls++; return n.name }

func readTypingUserName(t *testing.T, observer *Client) string {
	t.Helper()
	select {
	case b := <-observer.Send:
		var m struct {
			Event   string                    `json:"event"`
			Payload ConversationTypingPayload `json:"payload"`
		}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("payload hỏng: %v", err)
		}
		if m.Event != EventConversationTyping {
			t.Fatalf("muốn %s, nhận %s", EventConversationTyping, m.Event)
		}
		return m.Payload.UserName
	default:
		t.Fatal("người quan sát không nhận được sự kiện gõ")
		return ""
	}
}

func typingFrame(conv string, spoofed string) []byte {
	payload, _ := json.Marshal(ConversationTypingPayload{ConversationID: conv, UserName: spoofed, IsTyping: true})
	frame, _ := json.Marshal(IncomingMessage{Event: EventConversationTyping, Payload: payload})
	return frame
}

func newObserver(hub *Hub, channel string) *Client {
	o := &Client{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 8),
		authorizer: stubAuthorizer{allow: true}, channels: map[string]bool{}}
	o.Subscribe(channel)
	return o
}

func TestTypingRelay_Client_GanTenDoServerTra(t *testing.T) {
	hub := NewHub()
	conv := uuid.NewString()
	channel := "conversation:" + conv
	observer := newObserver(hub, channel)
	auth := &namingAuthorizer{stubAuthorizer: stubAuthorizer{allow: true}, name: "Lê Văn C"}
	c := &Client{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 8),
		authorizer: auth, channels: map[string]bool{channel: true}}

	c.handleMessage(typingFrame(conv, "Người Giả Mạo"))
	if got := readTypingUserName(t, observer); got != "Lê Văn C" {
		t.Fatalf("user_name = %q, muốn tên server tra", got)
	}
	c.handleMessage(typingFrame(conv, ""))
	readTypingUserName(t, observer)
	if auth.calls != 1 {
		t.Errorf("tên phải được nhớ theo kết nối: tra %d lần, muốn 1", auth.calls)
	}
}

func TestTypingRelay_FiberClient_GanTenDoServerTra(t *testing.T) {
	hub := NewHub()
	conv := uuid.NewString()
	channel := "conversation:" + conv
	observer := newObserver(hub, channel)
	auth := &namingAuthorizer{stubAuthorizer: stubAuthorizer{allow: true}, name: "Lê Văn C"}
	fc := &FiberClient{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 8),
		authorizer: auth, channels: map[string]bool{channel: true}}

	fc.handleMessage(typingFrame(conv, "Người Giả Mạo"))
	if got := readTypingUserName(t, observer); got != "Lê Văn C" {
		t.Fatalf("user_name = %q, muốn tên server tra", got)
	}
	// Ghim cache như Client: sự kiện gõ thứ hai trên cùng kết nối không tra lại tên (mỗi lần gõ là một truy vấn DB).
	fc.handleMessage(typingFrame(conv, ""))
	readTypingUserName(t, observer)
	if auth.calls != 1 {
		t.Errorf("tên phải được nhớ theo kết nối: tra %d lần, muốn 1", auth.calls)
	}
}

func TestTypingNameCache_KhongNhoTenRong(t *testing.T) {
	auth := &namingAuthorizer{stubAuthorizer: stubAuthorizer{allow: true}, name: ""}
	var cache typingNameCache
	id := uuid.New()
	cache.get(auth, id)
	cache.get(auth, id)
	if auth.calls != 2 {
		t.Fatalf("tên rỗng (lỗi tra) không được nhớ: tra %d lần, muốn 2", auth.calls)
	}
	auth.name = "An"
	if got := cache.get(auth, id); got != "An" {
		t.Fatalf("got %q", got)
	}
	cache.get(auth, id)
	if auth.calls != 3 {
		t.Fatalf("sau khi có tên phải ngừng tra: %d lần", auth.calls)
	}
}
