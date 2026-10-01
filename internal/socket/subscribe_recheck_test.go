package socket

// Review đối kháng #102 (MINOR 2): khe TOCTOU giữa kiểm quyền và đăng ký kênh. Nếu người dùng bị gỡ khỏi nhóm
// (Evict) NGAY SAU lần CanSubscribe đầu và TRƯỚC SubscribeToChannel thì kết nối được gắn vào kênh sau khi đã bị
// gỡ và nhận tin tới khi ngắt kết nối. Nay Subscribe kiểm lại quyền sau khi đăng ký và tự gỡ nếu hụt.

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// flipAuthorizer cho phép lần kiểm đầu tiên rồi từ chối mọi lần sau: giả lập "bị gỡ khỏi nhóm giữa hai lần kiểm".
type flipAuthorizer struct{ calls atomic.Int32 }

func (a *flipAuthorizer) CanSubscribe(uuid.UUID, string) (bool, error) {
	return a.calls.Add(1) == 1, nil
}

func hubHasClient(h *Hub, channel string, c *Client) bool {
	h.channelsMu.RLock()
	defer h.channelsMu.RUnlock()
	return h.channels[channel][c]
}

func TestSubscribe_BiGoGiuaHaiLanKiem_KhongConDangKy_Client(t *testing.T) {
	hub := NewHub()
	channel := "conversation:" + uuid.NewString()
	c := &Client{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 8),
		authorizer: &flipAuthorizer{}, channels: map[string]bool{}}
	c.Subscribe(channel)
	if hubHasClient(hub, channel, c) || c.IsSubscribed(channel) {
		t.Fatal("người đã mất quyền giữa hai lần kiểm vẫn còn đăng ký trong kênh")
	}
	select {
	case b := <-c.Send:
		if !strings.Contains(string(b), "subscribe_denied") {
			t.Errorf("muốn báo subscribe_denied, nhận %s", b)
		}
	default:
		t.Error("phải báo subscribe_denied cho client")
	}
}

func TestSubscribe_BiGoGiuaHaiLanKiem_KhongConDangKy_FiberClient(t *testing.T) {
	hub := NewHub()
	channel := "conversation:" + uuid.NewString()
	hc := &Client{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 8), channels: map[string]bool{}}
	fc := &FiberClient{ID: hc.ID, UserID: hc.UserID, Hub: hub, Send: make(chan []byte, 8),
		authorizer: &flipAuthorizer{}, channels: map[string]bool{}, hubClient: hc}
	fc.subscribe(channel)
	if hubHasClient(hub, channel, hc) {
		t.Fatal("người đã mất quyền giữa hai lần kiểm vẫn còn đăng ký trong kênh của hub")
	}
	fc.channelMu.RLock()
	still := fc.channels[channel]
	fc.channelMu.RUnlock()
	if still {
		t.Error("kênh không được còn trong danh sách của client")
	}
}

// Đối chứng: người còn quyền ở cả hai lần kiểm vẫn đăng ký bình thường.
func TestSubscribe_ConQuyen_VanDangKy(t *testing.T) {
	hub := NewHub()
	channel := "conversation:" + uuid.NewString()
	c := &Client{ID: uuid.New(), UserID: uuid.New(), Hub: hub, Send: make(chan []byte, 8),
		authorizer: stubAuthorizer{allow: true}, channels: map[string]bool{}}
	c.Subscribe(channel)
	if !hubHasClient(hub, channel, c) || !c.IsSubscribed(channel) {
		t.Fatal("thành viên còn quyền phải đăng ký được")
	}
}