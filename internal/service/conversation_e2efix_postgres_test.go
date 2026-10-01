package service

// Test Postgres THẬT cho ba lỗi tìm thấy khi kiểm chứng đầu-cuối nhóm/bạn bè (plans/reports/e2e-261001-groups-friends.md):
//   - F2: DM 1-1 báo cờ is_blocked ngay khi mở (GET detail, POST direct) để web khoá ô nhập, không lộ ai chặn ai;
//   - F3: người đã rời nhóm đọc tin nhận ErrNotParticipant (handler -> 404), cùng lỗi với hội thoại không tồn tại;
//   - F4: tin nhắn kèm sender_full_name để chat nhóm hiện họ tên như danh sách thành viên.
// Bỏ phần sửa tương ứng thì test ĐỎ.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func wantBlocked(t *testing.T, label string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: is_blocked vắng mặt, muốn %v", label, want)
		return
	}
	if *got != want {
		t.Errorf("%s: is_blocked=%v, muốn %v", label, *got, want)
	}
}

func mustGetConversation(t *testing.T, svc *ConversationService, uid, convID uuid.UUID) *dto.ConversationResponse {
	t.Helper()
	got, err := svc.GetConversation(t.Context(), uid, convID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// newGroupConversation tạo nhóm + hội thoại nhóm + participant cho các user truyền vào (ghi thẳng DB).
func newGroupConversation(t *testing.T, db *gorm.DB, owner uuid.UUID, users ...uuid.UUID) *model.Conversation {
	t.Helper()
	g := model.Group{Name: "E2EFIX nhom", Slug: "e2efix-" + uuid.NewString(), CreatedBy: owner, Privacy: model.GroupPrivacyPublic, MaxMembers: 10}
	if err := db.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	conv := model.Conversation{Type: model.ConversationTypeGroup, GroupID: &g.ID}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if err := db.Create(&model.ConversationParticipant{ConversationID: conv.ID, UserID: u}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return &conv
}

func TestDirectConversation_IsBlocked_HaiChieu_KhongLoAiChan(t *testing.T) {
	for _, tc := range []struct {
		name           string
		blocker, other int // 0 = a, 1 = b
	}{{"a chặn b", 0, 1}, {"b chặn a", 1, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			db := pgtestIsolated(t)
			svc := newConversationServiceWithFriends(db)
			ctx := t.Context()
			ab := [2]uuid.UUID{guardUser(t, db, "ib-a"), guardUser(t, db, "ib-b")}
			guardFriendship(t, db, ab[0], ab[1], model.FriendshipStatusAccepted)
			conv, err := svc.CreateDirectConversation(ctx, ab[0], ab[1])
			if err != nil {
				t.Fatal(err)
			}

			// Chưa chặn: cờ có mặt và là false (web phân biệt "đã biết không chặn" với "danh sách không tính").
			for i, uid := range ab {
				wantBlocked(t, "chưa chặn, người xem "+string(rune('a'+i)), mustGetConversation(t, svc, uid, conv.ID).IsBlocked, false)
			}

			guardBlock(t, db, ab[tc.blocker], ab[tc.other])

			seen := map[int]string{}
			for i, uid := range ab {
				got := mustGetConversation(t, svc, uid, conv.ID)
				wantBlocked(t, "GET detail, người xem "+string(rune('a'+i)), got.IsBlocked, true)
				b, _ := json.Marshal(got.IsBlocked)
				seen[i] = string(b)

				// POST /conversations/direct trả lại cuộc cũ: cùng cờ.
				again, err := svc.CreateDirectConversation(ctx, uid, ab[1-i])
				if err != nil {
					t.Fatal(err)
				}
				wantBlocked(t, "POST direct, người xem "+string(rune('a'+i)), again.IsBlocked, true)
			}
			// Hai phía thấy y hệt nhau: cờ không phụ thuộc ai là người chặn.
			if seen[0] != seen[1] {
				t.Errorf("hai phía phải thấy cờ giống nhau, nhận %q và %q", seen[0], seen[1])
			}
			// Không field nào nói ai chặn ai.
			full, _ := json.Marshal(mustGetConversation(t, svc, ab[tc.blocker], conv.ID))
			if s := strings.ToLower(string(full)); strings.Contains(s, "blocker") || strings.Contains(s, "blocked_by") {
				t.Errorf("response lộ ai chặn ai: %s", full)
			}

			// Bỏ chặn: cờ về false.
			db.Where("blocker_id = ? AND blocked_id = ?", ab[tc.blocker], ab[tc.other]).Delete(&model.UserBlock{})
			wantBlocked(t, "sau bỏ chặn", mustGetConversation(t, svc, ab[0], conv.ID).IsBlocked, false)
		})
	}
}

// Chặn chỉ có nghĩa với DM 1-1: chat nhóm không mang cờ dù hai thành viên chặn nhau.
func TestGroupConversation_KhongCoIsBlocked(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceWithFriends(db)
	a, b := guardUser(t, db, "gb-a"), guardUser(t, db, "gb-b")
	conv := newGroupConversation(t, db, a, a, b)
	guardBlock(t, db, a, b)
	if got := mustGetConversation(t, svc, a, conv.ID); got.IsBlocked != nil {
		t.Errorf("chat nhóm không được mang is_blocked, nhận %v", *got.IsBlocked)
	}
}

// F3: người đã rời, người chưa từng vào và hội thoại không tồn tại cùng nhận ErrNotParticipant, nên handler trả
// một 404 duy nhất (không lộ hội thoại có thật hay không). Người còn trong nhóm vẫn đọc được.
func TestListMessages_NguoiKhongConLaParticipant_ErrNotParticipant(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceForTest(db)
	ctx := t.Context()
	owner, leaver, outsider := guardUser(t, db, "np-o"), guardUser(t, db, "np-l"), guardUser(t, db, "np-x")
	conv := newGroupConversation(t, db, owner, owner, leaver)
	if _, err := svc.ListMessages(ctx, leaver, conv.ID, 1, 20); err != nil {
		t.Fatalf("tiền đề: thành viên đọc được: %v", err)
	}
	if err := svc.participantRepo.MarkLeft(ctx, conv.ID, leaver); err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]struct{ user, conv uuid.UUID }{
		"đã rời":             {leaver, conv.ID},
		"người ngoài":        {outsider, conv.ID},
		"hội thoại không có": {owner, uuid.New()},
	} {
		if _, err := svc.ListMessages(ctx, c.user, c.conv, 1, 20); !errors.Is(err, ErrNotParticipant) {
			t.Errorf("%s: muốn ErrNotParticipant, nhận %v", name, err)
		}
	}
	if _, err := svc.ListMessages(ctx, owner, conv.ID, 1, 20); err != nil {
		t.Errorf("chủ nhóm còn lại phải đọc được: %v", err)
	}
}

// F4: tin nhắn (gửi, danh sách, trả lời) mang họ tên người gửi; người chưa có họ tên thì bỏ trống để web rơi về user_name.
func TestMessage_SenderFullName(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceForTest(db)
	ctx := t.Context()
	named, unnamed := guardUser(t, db, "fn-n"), guardUser(t, db, "fn-u")
	if err := db.Model(&model.User{}).Where("id = ?", named).Update("full_name", "Phạm Thị D").Error; err != nil {
		t.Fatal(err)
	}
	conv := newGroupConversation(t, db, named, named, unnamed)

	text := func(s string) dto.SendMessageRequest { return dto.SendMessageRequest{Content: &s, Type: "TEXT"} }
	m1, err := svc.SendMessage(ctx, named, conv.ID, text("xin chào"))
	if err != nil {
		t.Fatal(err)
	}
	if m1.SenderFullName == nil || *m1.SenderFullName != "Phạm Thị D" {
		t.Errorf("tin vừa gửi (cũng là payload WebSocket) thiếu sender_full_name: %v", m1.SenderFullName)
	}
	reply := text("trả lời")
	reply.ReplyToID = &m1.ID
	m2, err := svc.SendMessage(ctx, unnamed, conv.ID, reply)
	if err != nil {
		t.Fatal(err)
	}
	if m2.SenderFullName != nil {
		t.Errorf("người chưa có họ tên phải để trống sender_full_name, nhận %q", *m2.SenderFullName)
	}
	if m2.ReplyTo == nil || m2.ReplyTo.SenderFullName == nil || *m2.ReplyTo.SenderFullName != "Phạm Thị D" {
		t.Errorf("dòng trả lời thiếu sender_full_name của tin gốc: %+v", m2.ReplyTo)
	}

	list, err := svc.ListMessages(ctx, named, conv.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]*string{}
	for _, m := range list.Messages {
		got[m.ID] = m.SenderFullName
	}
	if p := got[m1.ID]; p == nil || *p != "Phạm Thị D" {
		t.Errorf("danh sách tin thiếu sender_full_name: %v", p)
	}
	if got[m2.ID] != nil {
		t.Errorf("danh sách: người chưa có họ tên phải để trống")
	}

	// Họ tên toàn khoảng trắng coi như chưa có (không hiện ô tên trống).
	if err := db.Model(&model.User{}).Where("id = ?", unnamed).Update("full_name", "   ").Error; err != nil {
		t.Fatal(err)
	}
	m3, err := svc.SendMessage(ctx, unnamed, conv.ID, text("ba"))
	if err != nil {
		t.Fatal(err)
	}
	if m3.SenderFullName != nil {
		t.Errorf("họ tên toàn khoảng trắng phải coi như trống, nhận %q", *m3.SenderFullName)
	}
}
