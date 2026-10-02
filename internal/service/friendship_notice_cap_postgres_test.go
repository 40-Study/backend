package service

// Test Postgres THẬT cho trần thông báo lời mời kết bạn (L3, review gọi là NEW-4): gửi-huỷ-gửi mỗi 5 phút
// sinh ~288 thông báo/ngày cho một người. Bỏ claimRequestNotice (luôn true), đổi trần, quên reset khi hết
// cửa sổ hoặc khi đổi chiều thì các test dưới ĐỎ.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/model"
)

// requestNoticesTo đếm thông báo friend_request đã phát cho người nhận.
func requestNoticesTo(fx *friendFx, to uuid.UUID) int {
	n := 0
	for _, s := range fx.notif.all() {
		if s.NotificationType == model.NotificationTypeFriendRequest && len(s.UserIDs) == 1 && s.UserIDs[0] == to {
			n++
		}
	}
	return n
}

// cycle gửi rồi huỷ một lời mời (đẩy đồng hồ qua cooldown huỷ để lần sau gửi lại được); trả id lời mời.
func (fx *friendFx) cycle(clock *time.Time, from, to uuid.UUID) uuid.UUID {
	fx.t.Helper()
	id := fx.send(from, to)
	if err := fx.svc.CancelRequest(fx.t.Context(), from, id); err != nil {
		fx.t.Fatalf("CancelRequest: %v", err)
	}
	*clock = clock.Add(constants.FriendCancelCooldown + time.Minute)
	return id
}

func TestFriendship_TranThongBaoLoiMoi_MoiCapMoi24Gio(t *testing.T) {
	fx := newFriendFx(t)
	clock := time.Now()
	fx.svc.now = func() time.Time { return clock }
	a, b, c, d := fx.student("a"), fx.student("b"), fx.student("c"), fx.student("d")

	// 6 lần gửi-huỷ-gửi trong vài chục phút: chỉ FriendRequestNoticeLimit thông báo tới b.
	for i := 0; i < constants.FriendRequestNoticeLimit+3; i++ {
		fx.cycle(&clock, a, b)
	}
	if got := requestNoticesTo(fx, b); got != constants.FriendRequestNoticeLimit {
		t.Fatalf("b nhận %d thông báo, muốn đúng %d (trần mỗi cặp/24h)", got, constants.FriendRequestNoticeLimit)
	}

	// Vượt trần: lời mời VẪN tạo được (PENDING, hiện trong danh sách), chỉ không có thông báo.
	id := fx.send(a, b)
	if got := requestNoticesTo(fx, b); got != constants.FriendRequestNoticeLimit {
		t.Errorf("lời mời vượt trần không được đẩy thông báo: b có %d", got)
	}
	in, err := fx.svc.ListRequests(t.Context(), b, "incoming", 1, 20)
	if err != nil || in.TotalCount != 1 || in.Requests[0].ID != id {
		t.Fatalf("lời mời vượt trần phải hiện cho người nhận: %+v %v", in, err)
	}
	// Người nhận vẫn chấp nhận được lời mời không có thông báo.
	if _, err := fx.svc.AcceptRequest(t.Context(), b, id); err != nil {
		t.Errorf("chấp nhận lời mời vượt trần: %v", err)
	}

	// Trần theo CẶP: người khác gửi cho b, hay a gửi cho người khác, vẫn có thông báo.
	fx.send(c, b)
	if got := requestNoticesTo(fx, b); got != constants.FriendRequestNoticeLimit+1 {
		t.Errorf("người gửi khác phải có thông báo riêng: b có %d", got)
	}
	fx.send(a, d)
	if got := requestNoticesTo(fx, d); got != 1 {
		t.Errorf("a gửi cho người khác phải có thông báo: d có %d", got)
	}
}

func TestFriendship_TranThongBaoLoiMoi_HetCuaSoDemLai(t *testing.T) {
	fx := newFriendFx(t)
	clock := time.Now()
	fx.svc.now = func() time.Time { return clock }
	a, b := fx.student("a"), fx.student("b")

	for i := 0; i < constants.FriendRequestNoticeLimit+1; i++ {
		fx.cycle(&clock, a, b)
	}
	if got := requestNoticesTo(fx, b); got != constants.FriendRequestNoticeLimit {
		t.Fatalf("trước khi hết cửa sổ: b có %d, muốn %d", got, constants.FriendRequestNoticeLimit)
	}

	clock = clock.Add(constants.FriendRequestNoticeWindow + time.Minute)
	fx.send(a, b)
	if got := requestNoticesTo(fx, b); got != constants.FriendRequestNoticeLimit+1 {
		t.Errorf("sau 24h cửa sổ mới: thông báo phải lại được đẩy, b có %d", got)
	}
}

// Đổi chiều (người nhận cũ chủ động gửi) là cặp gửi→nhận khác nên có bộ đếm riêng; và việc đó không làm
// hỏng trần của chiều cũ khi họ quay lại gửi trong cùng cửa sổ.
func TestFriendship_TranThongBaoLoiMoi_DoiChieuDemLai(t *testing.T) {
	fx := newFriendFx(t)
	clock := time.Now()
	fx.svc.now = func() time.Time { return clock }
	a, b := fx.student("a"), fx.student("b")

	for i := 0; i < constants.FriendRequestNoticeLimit; i++ {
		fx.cycle(&clock, a, b)
	}
	// b chủ động gửi cho a: a phải nhận được thông báo (b chưa từng gửi cho a).
	fx.cycle(&clock, b, a)
	if got := requestNoticesTo(fx, a); got != 1 {
		t.Errorf("chiều b→a có bộ đếm riêng: a phải nhận 1 thông báo, có %d", got)
	}
}

// Bộ đếm phải là dữ liệu bền trên dòng cặp (sống qua huỷ và qua restart), không phải bộ nhớ tiến trình.
func TestFriendship_TranThongBaoLoiMoi_BoDemLuuTrenDong(t *testing.T) {
	fx := newFriendFx(t)
	a, b := fx.student("a"), fx.student("b")
	fx.send(a, b)
	row := fx.rowOf(a, b)
	if row.NoticeCount != 1 || row.NoticeWindowStart == nil {
		t.Fatalf("sau lần gửi đầu: count=%d window=%v, muốn count=1 và có cửa sổ", row.NoticeCount, row.NoticeWindowStart)
	}
}
