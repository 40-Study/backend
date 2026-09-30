package app

// Lane S6: authorizer WebSocket theo từng vai. Trước S6 mọi tài khoản đăng ký được mọi kênh conversation:/group:.
// Bỏ kiểm tra participant/member ở CanSubscribe thì các ca "người ngoài"/"đã rời" ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

type fakeParticipants struct {
	active map[[2]uuid.UUID]bool
	err    error
}

func (f fakeParticipants) IsActiveParticipant(_ context.Context, conv, user uuid.UUID) (bool, error) {
	return f.active[[2]uuid.UUID{conv, user}], f.err
}

type fakeGroupMembers struct{ active map[[2]uuid.UUID]bool }

func (f fakeGroupMembers) GetActiveByGroupAndUser(_ context.Context, g, u uuid.UUID) (*model.GroupMember, error) {
	if f.active[[2]uuid.UUID{g, u}] {
		return &model.GroupMember{GroupID: g, UserID: u}, nil
	}
	return nil, nil
}

func TestS6_WSAuthorizer_TheoTungVai(t *testing.T) {
	member, stranger, leaver := uuid.New(), uuid.New(), uuid.New()
	conv, grp := uuid.New(), uuid.New()
	// leaver từng là thành viên nhưng đã rời: không còn dòng active.
	a := newWSChannelAuthorizer(
		fakeParticipants{active: map[[2]uuid.UUID]bool{{conv, member}: true}},
		fakeGroupMembers{active: map[[2]uuid.UUID]bool{{grp, member}: true}},
	)
	cases := []struct {
		name    string
		user    uuid.UUID
		channel string
		want    bool
	}{
		{"thành viên nghe hội thoại", member, "conversation:" + conv.String(), true},
		{"người ngoài nghe hội thoại", stranger, "conversation:" + conv.String(), false},
		{"người đã rời nghe hội thoại", leaver, "conversation:" + conv.String(), false},
		{"thành viên nghe kênh nhóm", member, "group:" + grp.String(), true},
		{"người ngoài nghe kênh nhóm", stranger, "group:" + grp.String(), false},
		{"người đã rời nghe kênh nhóm", leaver, "group:" + grp.String(), false},
		{"nghe kênh user của chính mình", member, "user:" + member.String(), true},
		{"nghe kênh user của người khác", stranger, "user:" + member.String(), false},
		{"kênh notifications chung", stranger, "notifications", true},
		{"kênh lạ", member, "admin:all", false},
		{"id sai định dạng", member, "conversation:not-a-uuid", false},
		{"không có tiền tố", member, "conversation", false},
	}
	for _, c := range cases {
		got, err := a.CanSubscribe(c.user, c.channel)
		if err != nil || got != c.want {
			t.Errorf("%s: got=%v err=%v, muốn %v", c.name, got, err, c.want)
		}
	}
}

// Lỗi hạ tầng phải là "từ chối kèm lỗi", không bao giờ là cho qua.
func TestS6_WSAuthorizer_LoiHaTangThiTuChoi(t *testing.T) {
	boom := errors.New("db down")
	a := newWSChannelAuthorizer(fakeParticipants{err: boom}, fakeGroupMembers{})
	got, err := a.CanSubscribe(uuid.New(), "conversation:"+uuid.NewString())
	if got || !errors.Is(err, boom) {
		t.Fatalf("got=%v err=%v, muốn false + lỗi hạ tầng", got, err)
	}
}