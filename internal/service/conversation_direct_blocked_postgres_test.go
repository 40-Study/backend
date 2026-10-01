package service

// Test Postgres THẬT cho quyết định chủ dự án về review đối kháng #102 M2: chặn khoá gửi HAI CHIỀU trong DM 1-1
// đã có (gửi, sửa, xoá tin), lịch sử vẫn đọc được, bỏ chặn thì gửi lại được, chat nhóm không bị ảnh hưởng.
// Thay thế kỳ vọng cũ "DM đã có vẫn gửi được sau khi chặn" (chỉ còn kiểm CreateDirectConversation trả lại cuộc cũ).

import (
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func dmText(s string) dto.SendMessageRequest { return dto.SendMessageRequest{Content: &s, Type: "TEXT"} }

func TestDirectConversation_Chan_KhoaGuiHaiChieu_LichSuVanDoc_BoChanGuiLai(t *testing.T) {
	for _, tc := range []struct {
		name           string
		blocker, other int // 0 = a, 1 = b
	}{{"a chặn b", 0, 1}, {"b chặn a", 1, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			db := pgtestIsolated(t)
			svc := newConversationServiceWithFriends(db)
			ctx := t.Context()
			ab := [2]uuid.UUID{guardUser(t, db, "dm-a"), guardUser(t, db, "dm-b")}
			guardFriendship(t, db, ab[0], ab[1], model.FriendshipStatusAccepted)
			conv, err := svc.CreateDirectConversation(ctx, ab[0], ab[1])
			if err != nil {
				t.Fatal(err)
			}
			m1, err := svc.SendMessage(ctx, ab[0], conv.ID, dmText("trước khi chặn"))
			if err != nil {
				t.Fatalf("tiền đề: gửi khi chưa chặn phải được: %v", err)
			}

			guardBlock(t, db, ab[tc.blocker], ab[tc.other])
			for who, uid := range map[string]uuid.UUID{"a": ab[0], "b": ab[1]} {
				if _, err := svc.SendMessage(ctx, uid, conv.ID, dmText("sau khi chặn")); err != ErrConversationBlocked {
					t.Errorf("%s gửi tin vào DM đã bị chặn: muốn ErrConversationBlocked, nhận %v", who, err)
				}
			}
			if _, err := svc.EditMessage(ctx, ab[0], conv.ID, m1.ID, dto.EditMessageRequest{Content: "sửa"}); err != ErrConversationBlocked {
				t.Errorf("sửa tin trong DM bị chặn: muốn ErrConversationBlocked, nhận %v", err)
			}
			if err := svc.DeleteMessage(ctx, ab[0], conv.ID, m1.ID); err != ErrConversationBlocked {
				t.Errorf("xoá tin trong DM bị chặn: muốn ErrConversationBlocked, nhận %v", err)
			}
			// Lịch sử vẫn đọc được cho cả hai và tin cũ còn nguyên.
			for _, uid := range ab {
				list, err := svc.ListMessages(ctx, uid, conv.ID, 1, 20)
				if err != nil || list.TotalCount != 1 {
					t.Errorf("lịch sử phải đọc được: err=%v list=%+v", err, list)
				}
			}
			var n int64
			db.Model(&model.Message{}).Where("conversation_id = ?", conv.ID).Count(&n)
			if n != 1 {
				t.Errorf("tin bị chặn không được lưu: có %d tin", n)
			}

			// Bỏ chặn: gửi lại bình thường.
			db.Where("blocker_id = ? AND blocked_id = ?", ab[tc.blocker], ab[tc.other]).Delete(&model.UserBlock{})
			if _, err := svc.SendMessage(ctx, ab[1], conv.ID, dmText("sau khi bỏ chặn")); err != nil {
				t.Errorf("bỏ chặn phải gửi lại được: %v", err)
			}
		})
	}
}

// Chặn chỉ ảnh hưởng DM 1-1: hai người bị chặn nhau vẫn chat được trong nhóm chung.
func TestGroupConversation_Chan_KhongKhoaChatNhom(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceWithFriends(db)
	ctx := t.Context()
	a, b := guardUser(t, db, "gc-a"), guardUser(t, db, "gc-b")
	g := model.Group{Name: "QA nhom", Slug: "qa-" + uuid.NewString(), CreatedBy: a, Privacy: model.GroupPrivacyPublic, MaxMembers: 10}
	if err := db.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	conv := model.Conversation{Type: model.ConversationTypeGroup, GroupID: &g.ID}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	for _, u := range []uuid.UUID{a, b} {
		if err := db.Create(&model.ConversationParticipant{ConversationID: conv.ID, UserID: u}).Error; err != nil {
			t.Fatal(err)
		}
	}
	guardBlock(t, db, a, b)
	for who, uid := range map[string]uuid.UUID{"a": a, "b": b} {
		if _, err := svc.SendMessage(ctx, uid, conv.ID, dmText("chat nhóm")); err != nil {
			t.Errorf("%s: chat nhóm không được bị khoá bởi chặn: %v", who, err)
		}
	}
}
