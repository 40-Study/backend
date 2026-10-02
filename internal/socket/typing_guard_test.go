package socket

// typingAllowed (khoá "đang gõ" trong DM bị chặn): nhánh fail-closed. Contract hứa lỗi tra cứu và id sai định dạng
// đều BỎ sự kiện. Đổi `err == nil && allowed` thành `allowed`, hoặc bỏ nhánh uuid.Parse thì test ĐỎ.

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

type guardAuthorizer struct {
	allow bool
	err   error
	calls int
}

func (g *guardAuthorizer) CanSubscribe(uuid.UUID, string) (bool, error) { return true, nil }
func (g *guardAuthorizer) CanBroadcastTyping(uuid.UUID, uuid.UUID) (bool, error) {
	g.calls++
	return g.allow, g.err
}

func TestTypingAllowed(t *testing.T) {
	user, conv := uuid.New(), uuid.NewString()
	cases := []struct {
		name string
		a    ChannelAuthorizer
		conv string
		want bool
	}{
		{"guard cho phép", &guardAuthorizer{allow: true}, conv, true},
		{"guard chặn (DM bị chặn)", &guardAuthorizer{allow: false}, conv, false},
		{"guard lỗi hạ tầng: fail-closed dù allow=true", &guardAuthorizer{allow: true, err: errors.New("db down")}, conv, false},
		{"guard lỗi hạ tầng, allow=false", &guardAuthorizer{allow: false, err: errors.New("db down")}, conv, false},
		{"id hội thoại sai định dạng: bỏ, không hỏi guard", &guardAuthorizer{allow: true}, "khong-phai-uuid", false},
		{"authorizer không cài TypingGuard: chuyển tiếp như cũ", stubAuthorizer{allow: true}, conv, true},
		{"authorizer nil: chuyển tiếp như cũ", nil, conv, true},
	}
	for _, c := range cases {
		if got := typingAllowed(c.a, user, c.conv); got != c.want {
			t.Errorf("%s: muốn %v, nhận %v", c.name, c.want, got)
		}
	}
	g := &guardAuthorizer{allow: true}
	typingAllowed(g, user, "khong-phai-uuid")
	if g.calls != 0 {
		t.Error("id sai định dạng không được gọi guard")
	}
}
