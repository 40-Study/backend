package service

// Test Postgres THẬT cho vòng đời lời mời kết bạn (phase 01, plan 260930): gửi/chấp nhận/từ chối/thu hồi/
// huỷ bạn, chống trùng hai chiều, cooldown, hạn mức, trần 500 bạn, thông báo.

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestFriendship_GuiVaChapNhan_TroThanhBan(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")

	out, err := fx.svc.SendRequest(ctx, a, b)
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if out.AutoAccepted || out.Result.Status != model.FriendshipStatusPending || out.Result.User.UserID != b {
		t.Fatalf("muốn PENDING tới b, nhận %+v", out)
	}
	if ok, _ := fx.svc.AreFriends(ctx, a, b); ok {
		t.Fatal("PENDING không được coi là bạn")
	}

	acc, err := fx.svc.AcceptRequest(ctx, b, out.Result.ID)
	if err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}
	if acc.Status != model.FriendshipStatusAccepted || acc.User.UserID != a {
		t.Fatalf("muốn ACCEPTED với user=a, nhận %+v", acc)
	}
	for _, pair := range [][2]uuid.UUID{{a, b}, {b, a}} {
		if ok, _ := fx.svc.AreFriends(ctx, pair[0], pair[1]); !ok {
			t.Errorf("AreFriends(%v,%v) phải đối xứng và true sau khi chấp nhận", pair[0], pair[1])
		}
	}
	if r := fx.rowOf(a, b); r.RespondedAt == nil {
		t.Error("chấp nhận phải ghi responded_at")
	}
}

func TestFriendship_TuKetBan_Va_GuiTrung(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")

	_, err := fx.svc.SendRequest(ctx, a, a)
	wantErr(t, err, ErrFriendSelfRequest, "tự kết bạn")

	fx.send(a, b)
	_, err = fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestExists, "gửi lại khi đang chờ")
	if n := fx.countRows(a, b); n != 1 {
		t.Errorf("gửi trùng không được tạo thêm dòng, có %d", n)
	}

	fx.befriend(fx.student("c"), fx.student("d")) // không liên quan
	c, d := fx.student("c2"), fx.student("d2")
	fx.befriend(c, d)
	_, err = fx.svc.SendRequest(ctx, c, d)
	wantErr(t, err, ErrFriendAlreadyFriends, "đã là bạn (chiều gốc)")
	_, err = fx.svc.SendRequest(ctx, d, c)
	wantErr(t, err, ErrFriendAlreadyFriends, "đã là bạn (chiều ngược)")
}

// Đối phương đã gửi cho mình rồi mình gửi lại = tự chấp nhận (200), không tạo dòng mới.
func TestFriendship_GuiNguocChieu_TuChapNhan(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	reqID := fx.send(a, b)

	out, err := fx.svc.SendRequest(ctx, b, a)
	if err != nil {
		t.Fatalf("SendRequest ngược chiều: %v", err)
	}
	if !out.AutoAccepted || out.Result.Status != model.FriendshipStatusAccepted || out.Result.ID != reqID {
		t.Fatalf("muốn tự chấp nhận đúng dòng %v, nhận %+v", reqID, out)
	}
	if n := fx.countRows(a, b); n != 1 {
		t.Errorf("tự chấp nhận dùng lại dòng cũ, có %d dòng", n)
	}
	// Thông báo: a (người gửi gốc) nhận friend_accepted, không có friend_request thứ hai cho a.
	var toA []dto.CreateNotificationDTO
	for _, n := range fx.notif.all() {
		if n.UserIDs[0] == a {
			toA = append(toA, n)
		}
	}
	if len(toA) != 1 || toA[0].NotificationType != model.NotificationTypeFriendAccepted || *toA[0].ReferenceID != b {
		t.Errorf("a phải nhận đúng 1 friend_accepted tham chiếu b, nhận %+v", toA)
	}
}

// Hai người bấm gửi cho nhau CÙNG LÚC: chỉ một dòng, một bên PENDING rồi bên kia tự chấp nhận, không ai
// nhận lỗi thô. Bỏ LockUsers khỏi SendRequest thì bên đến sau vi phạm unique và nhận lỗi -> đỏ.
func TestFriendship_GuiDongThoiHaiChieu_ChiMotDong(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	for i := 0; i < 15; i++ {
		a, b := fx.student("ra"), fx.student("rb")
		start := make(chan struct{})
		var wg sync.WaitGroup
		outs := make([]*FriendSendOutcome, 2)
		errs := make([]error, 2)
		for k, pair := range [][2]uuid.UUID{{a, b}, {b, a}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				outs[k], errs[k] = fx.svc.SendRequest(ctx, pair[0], pair[1])
			}()
		}
		close(start)
		wg.Wait()

		for k := range errs {
			if errs[k] != nil {
				t.Fatalf("vòng %d: SendRequest #%d lỗi %v (muốn không lỗi: một bên PENDING, bên kia tự chấp nhận)", i, k, errs[k])
			}
		}
		auto := 0
		for _, o := range outs {
			if o.AutoAccepted {
				auto++
			}
		}
		if auto != 1 {
			t.Fatalf("vòng %d: muốn đúng 1 bên tự chấp nhận, có %d", i, auto)
		}
		if n := fx.countRows(a, b); n != 1 {
			t.Fatalf("vòng %d: muốn đúng 1 dòng cho cặp, có %d", i, n)
		}
		if r := fx.rowOf(a, b); r.Status != model.FriendshipStatusAccepted {
			t.Fatalf("vòng %d: muốn ACCEPTED, nhận %s", i, r.Status)
		}
	}
}

func TestFriendship_TuChoi_Cooldown7Ngay_RoiGuiLaiDuoc(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	reqID := fx.send(a, b)

	res, err := fx.svc.DeclineRequest(ctx, b, reqID)
	if err != nil || res.Status != model.FriendshipStatusDeclined {
		t.Fatalf("DeclineRequest: %v %+v", err, res)
	}
	if ok, _ := fx.svc.AreFriends(ctx, a, b); ok {
		t.Fatal("DECLINED không được coi là bạn")
	}

	_, err = fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestCooldown, "gửi lại ngay sau khi bị từ chối")

	// Còn 1 giờ nữa mới hết cooldown: vẫn bị chặn.
	fx.age(a, b, constants.FriendDeclineCooldown-time.Hour)
	_, err = fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestCooldown, "gửi lại khi cooldown chưa hết")

	fx.age(a, b, 2*time.Hour) // nay đã quá 7 ngày
	out, err := fx.svc.SendRequest(ctx, a, b)
	if err != nil {
		t.Fatalf("gửi lại sau cooldown: %v", err)
	}
	if out.Result.ID != reqID || out.Result.Status != model.FriendshipStatusPending {
		t.Errorf("gửi lại dùng lại dòng cũ với PENDING, nhận %+v", out.Result)
	}
	if r := fx.rowOf(a, b); r.RespondedAt != nil || r.RequesterID != a {
		t.Errorf("gửi lại phải xoá responded_at và giữ chiều a->b: %+v", r)
	}
}

// Cooldown chỉ ràng buộc NGƯỜI BỊ TỪ CHỐI; người đã từ chối muốn chủ động gửi thì được.
func TestFriendship_NguoiTuChoiChuDongGui_KhongBiCooldown(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	reqID := fx.send(a, b)
	if _, err := fx.svc.DeclineRequest(ctx, b, reqID); err != nil {
		t.Fatal(err)
	}

	out, err := fx.svc.SendRequest(ctx, b, a)
	if err != nil {
		t.Fatalf("người từ chối chủ động gửi: %v", err)
	}
	if out.AutoAccepted || out.Result.Status != model.FriendshipStatusPending {
		t.Fatalf("muốn PENDING b->a, nhận %+v", out)
	}
	if r := fx.rowOf(a, b); r.RequesterID != b || r.AddresseeID != a {
		t.Errorf("chiều phải đổi thành b->a: %+v", r)
	}
	// a nhận được lời mời của b và đồng ý.
	if _, err := fx.svc.AcceptRequest(ctx, a, out.Result.ID); err != nil {
		t.Errorf("a chấp nhận lời mời của b: %v", err)
	}
}

func TestFriendship_ThuHoi_AnKhoiMoiNguoi_NhungKhongLachHanMucVaCooldown(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	reqID := fx.send(a, b)

	if err := fx.svc.CancelRequest(ctx, a, reqID); err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}
	// Người nhận không còn thấy lời mời.
	_, err := fx.svc.AcceptRequest(ctx, b, reqID)
	wantErr(t, err, ErrFriendRequestNotFound, "chấp nhận lời mời đã thu hồi")
	_, err = fx.svc.DeclineRequest(ctx, b, reqID)
	wantErr(t, err, ErrFriendRequestNotFound, "từ chối lời mời đã thu hồi")
	rel, _ := fx.svc.Relationship(ctx, a, b)
	if rel.Status != dto.RelationNone {
		t.Errorf("sau thu hồi quan hệ phải là NONE, nhận %s", rel.Status)
	}
	in, _ := fx.svc.ListRequests(ctx, b, "incoming", 1, 20)
	out, _ := fx.svc.ListRequests(ctx, a, "outgoing", 1, 20)
	if in.TotalCount != 0 || out.TotalCount != 0 {
		t.Errorf("lời mời đã thu hồi không được xuất hiện trong danh sách: in=%d out=%d", in.TotalCount, out.TotalCount)
	}
	// Thu hồi lần hai: không còn lời mời.
	wantErr(t, fx.svc.CancelRequest(ctx, a, reqID), ErrFriendRequestNotFound, "thu hồi hai lần")

	// Gửi-huỷ-gửi liên tục bị chặn bằng cooldown thu hồi (chống quấy rối một người).
	_, err = fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestCooldown, "gửi lại ngay sau khi thu hồi")
	fx.age(a, b, constants.FriendCancelCooldown+time.Minute)
	if _, err := fx.svc.SendRequest(ctx, a, b); err != nil {
		t.Errorf("gửi lại sau cooldown thu hồi: %v", err)
	}
}

// Hạn mức 20 lời mời/24h tính cả lời mời đã thu hồi; lời mời cũ hơn 24h thì không tính.
func TestFriendship_HanMuc20Moi24Gio_ThuHoiKhongHoanLai(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	targets := make([]uuid.UUID, 0, constants.FriendDailyRequestLimit+1)
	for i := 0; i < constants.FriendDailyRequestLimit+1; i++ {
		targets = append(targets, fx.student("t"))
	}
	ids := make([]uuid.UUID, constants.FriendDailyRequestLimit)
	for i := 0; i < constants.FriendDailyRequestLimit; i++ {
		ids[i] = fx.send(me, targets[i])
	}
	extra := targets[constants.FriendDailyRequestLimit]
	_, err := fx.svc.SendRequest(ctx, me, extra)
	wantErr(t, err, ErrFriendDailyLimit, "lời mời thứ 21 trong 24h")

	for _, id := range ids {
		if err := fx.svc.CancelRequest(ctx, me, id); err != nil {
			t.Fatal(err)
		}
	}
	_, err = fx.svc.SendRequest(ctx, me, extra)
	wantErr(t, err, ErrFriendDailyLimit, "thu hồi hết vẫn không được gửi thêm (không lách hạn mức)")

	// Quá 24 giờ thì hết tính.
	fx.db.Model(&model.Friendship{}).Where("requester_id = ?", me).
		Update("requested_at", time.Now().Add(-constants.FriendDailyWindow-time.Minute))
	if _, err := fx.svc.SendRequest(ctx, me, extra); err != nil {
		t.Errorf("sau 24h phải gửi được: %v", err)
	}
}

func TestFriendship_HanMuc30DangCho(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	old := time.Now().Add(-48 * time.Hour) // ngoài cửa sổ 24h để chỉ hạn mức "đang chờ" phát huy
	for i := 0; i < constants.FriendPendingRequestLimit; i++ {
		f := model.Friendship{RequesterID: me, AddresseeID: fx.student("p"), Status: model.FriendshipStatusPending, RequestedAt: old}
		if err := fx.db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
	}
	_, err := fx.svc.SendRequest(ctx, me, fx.student("next"))
	wantErr(t, err, ErrFriendPendingLimit, "lời mời chờ thứ 31")

	// Lời mời ĐẾN mình không làm đầy hạn mức gửi của mình.
	other := fx.student("other")
	fx.send(fx.student("x"), other)
	if _, err := fx.svc.SendRequest(ctx, other, fx.student("y")); err != nil {
		t.Errorf("hạn mức đang chờ chỉ tính lời mời đã GỬI: %v", err)
	}
}

func TestFriendship_TranBan500_HaiPhia(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	full, lonely, fresh := fx.student("full"), fx.student("lonely"), fx.student("fresh")

	users := make([]model.User, constants.FriendMaxFriends)
	for i := range users {
		users[i] = model.User{Email: "cap-" + uuid.NewString() + "@40study.test", PasswordHash: "x", UserName: "cap" + uuid.NewString()[:8]}
	}
	if err := fx.db.CreateInBatches(&users, 100).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := make([]model.Friendship, len(users))
	for i, u := range users {
		rows[i] = model.Friendship{RequesterID: full, AddresseeID: u.ID, Status: model.FriendshipStatusAccepted, RequestedAt: now, RespondedAt: &now}
	}
	if err := fx.db.CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatal(err)
	}

	_, err := fx.svc.SendRequest(ctx, full, lonely)
	wantErr(t, err, ErrFriendLimitReached, "người gửi đã đủ 500 bạn")
	_, err = fx.svc.SendRequest(ctx, lonely, full)
	wantErr(t, err, ErrFriendLimitReached, "người nhận đã đủ 500 bạn")

	// Chấp nhận khi người nhận đã đủ 500 (lời mời có từ trước).
	pending := model.Friendship{RequesterID: fresh, AddresseeID: full, Status: model.FriendshipStatusPending, RequestedAt: now}
	if err := fx.db.Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	_, err = fx.svc.AcceptRequest(ctx, full, pending.ID)
	wantErr(t, err, ErrFriendLimitReached, "chấp nhận khi đã đủ 500 bạn")
	// Tự chấp nhận (gửi ngược chiều) cũng phải giữ trần.
	_, err = fx.svc.SendRequest(ctx, full, fresh)
	wantErr(t, err, ErrFriendLimitReached, "tự chấp nhận khi đã đủ 500 bạn")
	if r := fx.rowOf(full, fresh); r.Status != model.FriendshipStatusPending {
		t.Errorf("vượt trần thì dòng phải giữ PENDING, nhận %s", r.Status)
	}
}

func TestFriendship_ChiNguoiNhanDuocChapNhanTuChoi_ChiNguoiGuiDuocThuHoi(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b, stranger := fx.student("a"), fx.student("b"), fx.student("stranger")
	reqID := fx.send(a, b)

	_, err := fx.svc.AcceptRequest(ctx, a, reqID)
	wantErr(t, err, ErrFriendRequestNotFound, "người GỬI tự chấp nhận")
	_, err = fx.svc.AcceptRequest(ctx, stranger, reqID)
	wantErr(t, err, ErrFriendRequestNotFound, "người lạ chấp nhận")
	_, err = fx.svc.DeclineRequest(ctx, stranger, reqID)
	wantErr(t, err, ErrFriendRequestNotFound, "người lạ từ chối")
	_, err = fx.svc.DeclineRequest(ctx, a, reqID)
	wantErr(t, err, ErrFriendRequestNotFound, "người gửi từ chối lời mời của chính mình")
	wantErr(t, fx.svc.CancelRequest(ctx, b, reqID), ErrFriendRequestNotFound, "người NHẬN thu hồi")
	wantErr(t, fx.svc.CancelRequest(ctx, stranger, reqID), ErrFriendRequestNotFound, "người lạ thu hồi")
	_, err = fx.svc.AcceptRequest(ctx, b, uuid.New())
	wantErr(t, err, ErrFriendRequestNotFound, "id không tồn tại")
	if r := fx.rowOf(a, b); r.Status != model.FriendshipStatusPending {
		t.Fatalf("các lần thử trái phép không được đổi dòng, nhận %s", r.Status)
	}

	// Không còn chờ: chấp nhận lần hai, từ chối rồi chấp nhận, thu hồi lời mời đã thành bạn.
	if _, err := fx.svc.AcceptRequest(ctx, b, reqID); err != nil {
		t.Fatal(err)
	}
	_, err = fx.svc.AcceptRequest(ctx, b, reqID)
	wantErr(t, err, ErrFriendRequestNotPending, "chấp nhận hai lần")
	_, err = fx.svc.DeclineRequest(ctx, b, reqID)
	wantErr(t, err, ErrFriendRequestNotPending, "từ chối lời mời đã chấp nhận")
	wantErr(t, fx.svc.CancelRequest(ctx, a, reqID), ErrFriendRequestNotPending, "thu hồi lời mời đã chấp nhận")
	if ok, _ := fx.svc.AreFriends(ctx, a, b); !ok {
		t.Error("thu hồi lời mời đã chấp nhận không được phá tình bạn")
	}
}

func TestFriendship_HuyKetBan(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b, c := fx.student("a"), fx.student("b"), fx.student("c")
	fx.befriend(a, b)

	wantErr(t, fx.svc.Unfriend(ctx, a, c), ErrFriendNotFound, "huỷ bạn với người chưa là bạn")
	fx.send(a, c) // chỉ PENDING, chưa là bạn
	wantErr(t, fx.svc.Unfriend(ctx, a, c), ErrFriendNotFound, "huỷ bạn khi mới PENDING")
	if fx.rowOf(a, c) == nil {
		t.Fatal("Unfriend không được xoá lời mời PENDING")
	}

	if err := fx.svc.Unfriend(ctx, b, a); err != nil { // chiều bất kỳ
		t.Fatalf("Unfriend: %v", err)
	}
	if fx.rowOf(a, b) != nil {
		t.Error("huỷ kết bạn phải xoá hẳn dòng")
	}
	wantErr(t, fx.svc.Unfriend(ctx, a, b), ErrFriendNotFound, "huỷ bạn lần hai")
	// Kết bạn lại được ngay (dòng đã xoá hẳn, không kẹt unique).
	if _, err := fx.svc.SendRequest(ctx, a, b); err != nil {
		t.Errorf("kết bạn lại sau khi huỷ: %v", err)
	}
}

func TestFriendship_ThongBao(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")

	reqID := fx.send(a, b)
	got := fx.notif.all()
	if len(got) != 1 {
		t.Fatalf("gửi lời mời phải phát đúng 1 thông báo, có %d", len(got))
	}
	n := got[0]
	if n.UserIDs[0] != b || n.NotificationType != model.NotificationTypeFriendRequest ||
		n.ReferenceType == nil || *n.ReferenceType != "friendship" || *n.ReferenceID != reqID {
		t.Errorf("friend_request sai: %+v", n)
	}

	if _, err := fx.svc.AcceptRequest(ctx, b, reqID); err != nil {
		t.Fatal(err)
	}
	got = fx.notif.all()
	if len(got) != 2 {
		t.Fatalf("chấp nhận phải phát thêm 1 thông báo, tổng %d", len(got))
	}
	n = got[1]
	if n.UserIDs[0] != a || n.NotificationType != model.NotificationTypeFriendAccepted ||
		n.ReferenceType == nil || *n.ReferenceType != "user" || *n.ReferenceID != b {
		t.Errorf("friend_accepted sai (phải gửi cho người gửi, tham chiếu người chấp nhận): %+v", n)
	}

	// Từ chối và thu hồi không phát thông báo (không nói cho người gửi biết bị từ chối).
	c, d := fx.student("c"), fx.student("d")
	id := fx.send(c, d)
	before := len(fx.notif.all())
	if _, err := fx.svc.DeclineRequest(ctx, d, id); err != nil {
		t.Fatal(err)
	}
	if len(fx.notif.all()) != before {
		t.Error("từ chối không được phát thông báo")
	}
}

// Gửi thông báo lỗi chỉ log: lời mời vẫn được tạo.
func TestFriendship_LoiGuiThongBao_KhongLamHongThaoTac(t *testing.T) {
	fx := newFriendFx(t)
	fx.notif.err = errTest
	a, b := fx.student("a"), fx.student("b")
	out, err := fx.svc.SendRequest(t.Context(), a, b)
	if err != nil || out.Result.Status != model.FriendshipStatusPending {
		t.Fatalf("lỗi thông báo làm hỏng lời mời: %v %+v", err, out)
	}
	if fx.rowOf(a, b) == nil {
		t.Error("dòng lời mời phải được giữ")
	}
	// Chưa nối notifier: cũng không lỗi.
	fx.svc.SetNotifier(nil)
	if _, err := fx.svc.SendRequest(t.Context(), a, fx.student("c")); err != nil {
		t.Errorf("thiếu notifier không được làm hỏng: %v", err)
	}
}

// Chỉ ACCEPTED là bạn (FriendshipChecker dùng cho DM guard/hồ sơ): PENDING/DECLINED/CANCELLED thì không.
func TestFriendship_AreFriends_ChiNhanAccepted(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	for _, status := range model.FriendshipStatuses {
		a, b := fx.student("a"), fx.student("b")
		now := time.Now()
		f := model.Friendship{RequesterID: a, AddresseeID: b, Status: status, RequestedAt: now, RespondedAt: &now}
		if err := fx.db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
		want := status == model.FriendshipStatusAccepted
		for _, pair := range [][2]uuid.UUID{{a, b}, {b, a}} {
			if got, err := fx.svc.AreFriends(ctx, pair[0], pair[1]); err != nil || got != want {
				t.Errorf("AreFriends với status %s = %v (err %v), muốn %v", status, got, err, want)
			}
		}
	}
	if got, _ := fx.svc.AreFriends(ctx, fx.student("x"), fx.student("y")); got {
		t.Error("hai người không có dòng nào không được là bạn")
	}
}
