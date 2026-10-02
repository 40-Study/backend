package service

// Test Postgres THẬT cho nhánh (c) "bạn bè" và chặn của guard tạo cuộc trò chuyện trực tiếp (plan 260930,
// phase 05), đảo kỳ vọng cũ "(c) luôn false vì chưa có bảng friendship". Người lạ vẫn bị chặn (bằng chứng guard
// còn tác dụng) — xem TestCreateDirectConversation_NguoiLa_Tra403 trong conversation_direct_guard_postgres_test.go.

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func pgtestIsolated(t *testing.T) *gorm.DB { return isolatedAPISchema(t) }

// newConversationServiceWithFriends nối FriendshipService thật (cùng DB) làm FriendshipChecker.
func newConversationServiceWithFriends(db *gorm.DB) *ConversationService {
	svc := newConversationServiceForTest(db)
	svc.SetFriendshipChecker(NewFriendshipService(repository.NewFriendshipRepository(db), repository.NewUserBlockRepository(db)))
	return svc
}

func guardFriendship(t *testing.T, db *gorm.DB, requester, addressee uuid.UUID, status string) {
	t.Helper()
	now := time.Now()
	f := model.Friendship{RequesterID: requester, AddresseeID: addressee, Status: status, RequestedAt: now, RespondedAt: &now}
	if err := db.Create(&f).Error; err != nil {
		t.Fatalf("tạo friendship %s: %v", status, err)
	}
}

func guardBlock(t *testing.T, db *gorm.DB, blocker, blocked uuid.UUID) {
	t.Helper()
	if err := db.Create(&model.UserBlock{BlockerID: blocker, BlockedID: blocked}).Error; err != nil {
		t.Fatalf("tạo block: %v", err)
	}
}

// Bạn bè ACCEPTED tạo được cuộc trò chuyện ở CẢ HAI chiều. Bỏ nhánh (c) thì đỏ.
func TestCreateDirectConversation_BanBeAccepted_ChoTao_HaiChieu(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceWithFriends(db)
	ctx := t.Context()

	// Hai cặp độc lập để thử cả hai chiều tạo (cuộc đã có thì không đi qua guard nữa).
	a, b := guardUser(t, db, "fr-a"), guardUser(t, db, "fr-b")
	c, d := guardUser(t, db, "fr-c"), guardUser(t, db, "fr-d")
	guardFriendship(t, db, a, b, model.FriendshipStatusAccepted) // a mời, a tạo
	guardFriendship(t, db, d, c, model.FriendshipStatusAccepted) // d mời, c (người nhận) tạo
	if _, err := svc.CreateDirectConversation(ctx, a, b); err != nil {
		t.Errorf("người gửi lời mời tạo cuộc với bạn: muốn cho phép, lỗi %v", err)
	}
	if _, err := svc.CreateDirectConversation(ctx, c, d); err != nil {
		t.Errorf("người nhận lời mời tạo cuộc với bạn: muốn cho phép, lỗi %v", err)
	}
	if ok, err := svc.CanStartDirectConversation(ctx, a, b); err != nil || !ok {
		t.Errorf("CanStartDirectConversation (dùng cho guard mời nhóm) muốn true, nhận %v %v", ok, err)
	}
}

// PENDING/DECLINED/CANCELLED KHÔNG phải bạn: nhận nhầm là rò quyền nghiêm trọng (mở DM cho người lạ). Nhận
// nhầm sẽ làm vòng lặp này ĐỎ; message nêu rõ trạng thái nào bị mở nhầm.
func TestCreateDirectConversation_KhongPhaiBan_Tra403(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceWithFriends(db)
	ctx := t.Context()

	for _, status := range []string{model.FriendshipStatusPending, model.FriendshipStatusDeclined, model.FriendshipStatusCancelled} {
		a, b := guardUser(t, db, "np-a"), guardUser(t, db, "np-b")
		guardFriendship(t, db, a, b, status)
		for _, pair := range [][2]uuid.UUID{{a, b}, {b, a}} {
			_, err := svc.CreateDirectConversation(ctx, pair[0], pair[1])
			if !errors.Is(err, ErrConversationNotAllowed) {
				t.Errorf("dòng friendships %s KHÔNG được mở DM, nhận err=%v", status, err)
			}
		}
	}
}

// Chặn thắng mọi nhánh trừ admin: bạn ACCEPTED (dữ liệu không nhất quán) hoặc học viên-giảng viên mà có block
// cũng không tạo được; admin vẫn qua.
func TestCreateDirectConversation_Chan_ThangMoiNhanhTruAdmin(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceWithFriends(db)
	ctx := t.Context()

	a, b := guardUser(t, db, "bl-a"), guardUser(t, db, "bl-b")
	guardFriendship(t, db, a, b, model.FriendshipStatusAccepted)
	guardBlock(t, db, a, b)
	for _, pair := range [][2]uuid.UUID{{a, b}, {b, a}} {
		_, err := svc.CreateDirectConversation(ctx, pair[0], pair[1])
		if !errors.Is(err, ErrConversationNotAllowed) {
			t.Errorf("bạn bè có block (chiều %v->%v): muốn ErrConversationNotAllowed, nhận %v", pair[0], pair[1], err)
		}
	}

	student, teacher := guardUser(t, db, "bl-student"), guardUser(t, db, "bl-teacher")
	guardEnroll(t, db, student, teacher)
	guardBlock(t, db, student, teacher)
	if _, err := svc.CreateDirectConversation(ctx, teacher, student); !errors.Is(err, ErrConversationNotAllowed) {
		t.Errorf("học viên-giảng viên có block: muốn ErrConversationNotAllowed, nhận %v", err)
	}

	admin, blockedByAdmin := guardUser(t, db, "bl-admin"), guardUser(t, db, "bl-target")
	guardMakeAdmin(t, db, admin)
	guardBlock(t, db, blockedByAdmin, admin)
	if _, err := svc.CreateDirectConversation(ctx, admin, blockedByAdmin); err != nil {
		t.Errorf("admin đi qua mọi nhánh kể cả khi có block: %v", err)
	}
}

// Chưa nối checker (nil) = đóng: cặp bạn bè thật cũng không tạo được, không mở cửa ngầm.
func TestCreateDirectConversation_ThieuChecker_DongKhongMoCua(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceForTest(db) // KHÔNG SetFriendshipChecker
	a, b := guardUser(t, db, "nc-a"), guardUser(t, db, "nc-b")
	guardFriendship(t, db, a, b, model.FriendshipStatusAccepted)
	if _, err := svc.CreateDirectConversation(t.Context(), a, b); !errors.Is(err, ErrConversationNotAllowed) {
		t.Errorf("thiếu checker: muốn ErrConversationNotAllowed, nhận %v", err)
	}
}

// Guard chỉ áp dụng khi TẠO MỚI: cuộc trực tiếp đã có vẫn mở (xem lịch sử) sau khi huỷ bạn hoặc chặn. Còn GỬI
// tin: huỷ bạn không đổi gì, nhưng CHẶN khoá gửi hai chiều (ErrConversationBlocked) — quyết định chủ dự án sau
// review đối kháng #102 M2, phủ đầy đủ ở conversation_direct_blocked_postgres_test.go.
func TestCreateDirectConversation_CuocDaCo_VanMoSauKhiHuyBanHoacChan(t *testing.T) {
	db := pgtestIsolated(t)
	svc := newConversationServiceWithFriends(db)
	ctx := t.Context()
	a, b := guardUser(t, db, "kl-a"), guardUser(t, db, "kl-b")
	guardFriendship(t, db, a, b, model.FriendshipStatusAccepted)
	first, err := svc.CreateDirectConversation(ctx, a, b)
	if err != nil {
		t.Fatal(err)
	}
	db.Where("requester_id = ? AND addressee_id = ?", a, b).Delete(&model.Friendship{})
	guardBlock(t, db, b, a)
	again, err := svc.CreateDirectConversation(ctx, a, b)
	if err != nil || again.ID != first.ID {
		t.Errorf("cuộc đã có phải vẫn mở và trả lại đúng cuộc cũ: err=%v again=%v first=%v", err, again, first)
	}
	// Hành vi MỚI: đã chặn (b chặn a ở trên) thì không gửi được tin mới vào cuộc cũ, ở cả hai phía.
	for who, uid := range map[string]uuid.UUID{"a": a, "b": b} {
		if _, err := svc.SendMessage(ctx, uid, first.ID, dmText("sau khi chặn")); !errors.Is(err, ErrConversationBlocked) {
			t.Errorf("%s gửi vào DM cũ sau khi bị chặn: muốn ErrConversationBlocked, nhận %v", who, err)
		}
	}
}
