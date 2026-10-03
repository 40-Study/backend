package service

// Lane R2 (QA hồi quy 03/10/2026, A-14), Postgres thật: thông báo "X đã gửi cho bạn lời mời" phải biến mất khi lời
// mời không còn hiệu lực (huỷ, từ chối, chấp nhận, chặn). Trước đây nó nằm lại và bấm vào dẫn tới tab Lời mời trống.
// Bỏ lời gọi cleanResolvedRequestNotices ở đường nào thì ca tương ứng ĐỎ; ca "lời mời còn chờ" chứng minh bộ dọn
// không xoá nhầm thông báo còn hiệu lực.

import (
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// seedRequestNotice tạo thông báo lời mời thật (fakeFriendNotifier không ghi DB) như SendRequest làm.
func (fx *friendFx) seedRequestNotice(to, requestID uuid.UUID) {
	fx.t.Helper()
	refType := "friendship"
	n := model.Notification{
		UserID: to, Title: "Lời mời kết bạn", Content: "đã gửi cho bạn lời mời kết bạn.",
		NotificationType: model.NotificationTypeFriendRequest, ReferenceType: &refType, ReferenceID: &requestID,
	}
	if err := fx.db.Create(&n).Error; err != nil {
		fx.t.Fatalf("tạo thông báo lời mời: %v", err)
	}
}

func (fx *friendFx) requestNotices(to uuid.UUID) int64 {
	fx.t.Helper()
	var n int64
	fx.db.Model(&model.Notification{}).
		Where("user_id = ? AND notification_type = ?", to, model.NotificationTypeFriendRequest).Count(&n)
	return n
}

func newFriendFxWithCleaner(t *testing.T) *friendFx {
	t.Helper()
	fx := newFriendFx(t)
	fx.svc.SetNoticeCleaner(repository.NewNotificationRepository(fx.db))
	return fx
}

func TestR2_ThongBaoLoiMoi_BienMatKhiLoiMoiHetHieuLuc(t *testing.T) {
	ctx := t.Context()

	t.Run("người gửi huỷ lời mời", func(t *testing.T) {
		fx := newFriendFxWithCleaner(t)
		a, b := fx.student("a"), fx.student("b")
		req := fx.send(a, b)
		fx.seedRequestNotice(b, req)
		if err := fx.svc.CancelRequest(ctx, a, req); err != nil {
			t.Fatal(err)
		}
		if n := fx.requestNotices(b); n != 0 {
			t.Errorf("sau khi huỷ, người nhận còn %d thông báo lời mời", n)
		}
	})

	t.Run("người nhận từ chối", func(t *testing.T) {
		fx := newFriendFxWithCleaner(t)
		a, b := fx.student("a"), fx.student("b")
		req := fx.send(a, b)
		fx.seedRequestNotice(b, req)
		if _, err := fx.svc.DeclineRequest(ctx, b, req); err != nil {
			t.Fatal(err)
		}
		if n := fx.requestNotices(b); n != 0 {
			t.Errorf("sau khi từ chối, còn %d thông báo lời mời", n)
		}
	})

	t.Run("người nhận chấp nhận", func(t *testing.T) {
		fx := newFriendFxWithCleaner(t)
		a, b := fx.student("a"), fx.student("b")
		req := fx.send(a, b)
		fx.seedRequestNotice(b, req)
		if _, err := fx.svc.AcceptRequest(ctx, b, req); err != nil {
			t.Fatal(err)
		}
		if n := fx.requestNotices(b); n != 0 {
			t.Errorf("sau khi chấp nhận, còn %d thông báo lời mời", n)
		}
	})

	t.Run("người gửi chặn người nhận khi lời mời còn chờ", func(t *testing.T) {
		fx := newFriendFxWithCleaner(t)
		a, b := fx.student("a"), fx.student("b")
		req := fx.send(a, b)
		fx.seedRequestNotice(b, req)
		if err := fx.svc.Block(ctx, a, b); err != nil {
			t.Fatal(err)
		}
		if n := fx.requestNotices(b); n != 0 {
			t.Errorf("sau khi chặn, còn %d thông báo lời mời", n)
		}
	})

	t.Run("gửi chéo: A gửi B rồi B gửi lại A thì lời mời của A được chấp nhận tự động", func(t *testing.T) {
		fx := newFriendFxWithCleaner(t)
		a, b := fx.student("a"), fx.student("b")
		req := fx.send(a, b)
		fx.seedRequestNotice(b, req)
		fx.send(b, a)
		if n := fx.requestNotices(b); n != 0 {
			t.Errorf("sau khi tự chấp nhận, B còn %d thông báo lời mời của A", n)
		}
	})

	t.Run("lời mời còn chờ giữ nguyên thông báo, và chỉ động tới người liên quan", func(t *testing.T) {
		fx := newFriendFxWithCleaner(t)
		a, b, c := fx.student("a"), fx.student("b"), fx.student("c")
		reqAB := fx.send(a, b)
		fx.seedRequestNotice(b, reqAB)
		reqCB := fx.send(c, b)
		fx.seedRequestNotice(b, reqCB)
		if err := fx.svc.CancelRequest(ctx, a, reqAB); err != nil {
			t.Fatal(err)
		}
		if n := fx.requestNotices(b); n != 1 {
			t.Errorf("B muốn còn đúng 1 thông báo (lời mời của C vẫn chờ), còn %d", n)
		}
	})

	t.Run("chưa nối bộ dọn thì không lỗi (chỉ mất tính gọn)", func(t *testing.T) {
		fx := newFriendFx(t)
		a, b := fx.student("a"), fx.student("b")
		req := fx.send(a, b)
		fx.seedRequestNotice(b, req)
		if err := fx.svc.CancelRequest(ctx, a, req); err != nil {
			t.Fatalf("huỷ khi chưa nối bộ dọn: %v", err)
		}
	})
}
