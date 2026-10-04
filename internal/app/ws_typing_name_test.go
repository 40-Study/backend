package app

// A-15: tên người gõ do server tra (họ tên -> tên đăng nhập -> rỗng khi lỗi/chưa nối).

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

type fakeNameLookup struct {
	user *model.User
	err  error
}

func (f fakeNameLookup) FindUserByID(context.Context, uuid.UUID) (*model.User, error) {
	return f.user, f.err
}

func TestWSAuthorizer_TypingDisplayName(t *testing.T) {
	full := "  Lê Văn C "
	blank := "   "
	id := uuid.New()
	cases := []struct {
		name   string
		lookup userNameLookup
		want   string
	}{
		{"chưa nối tra cứu", nil, ""},
		{"có họ tên", fakeNameLookup{user: &model.User{UserName: "student1", FullName: &full}}, "Lê Văn C"},
		{"họ tên trống rơi về username", fakeNameLookup{user: &model.User{UserName: "student1", FullName: &blank}}, "student1"},
		{"không có họ tên", fakeNameLookup{user: &model.User{UserName: "student1"}}, "student1"},
		{"lỗi tra cứu", fakeNameLookup{err: errors.New("db down")}, ""},
		{"không thấy user", fakeNameLookup{}, ""},
	}
	for _, c := range cases {
		a := newWSChannelAuthorizer(nil, nil)
		if c.lookup != nil {
			a.SetUserNameLookup(c.lookup)
		}
		if got := a.TypingDisplayName(id); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
