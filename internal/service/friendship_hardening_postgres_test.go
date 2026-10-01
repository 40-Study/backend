package service

// Test Postgres THẬT cho các lỗ do review đối kháng #101 chỉ ra (plans/reports/review-261001-lane-gf-be.md):
// M1 (chặn rồi bỏ chặn xoá cooldown và hạn mức), MINOR 1 (huỷ/từ chối không khoá), 3 (người bị chặn đoán ra
// mình bị chặn), 4 (lọc vai trò denylist), 5 (tìm kiếm lộ phần trước @ của email).

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/model"
)

// M1: dòng DECLINED còn cooldown 7 ngày; chặn rồi bỏ chặn KHÔNG được xoá nó. Trước khi sửa, Block xoá mọi dòng
// của cặp nên lời mời gửi lại được ngay và người bị từ chối nhận thông báo lần nữa.
func TestFriendship_Chan_BoChan_KhongXoaCooldownTuChoi(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	id := fx.send(a, b)
	if _, err := fx.svc.DeclineRequest(ctx, b, id); err != nil {
		t.Fatal(err)
	}
	_, err := fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestCooldown, "tiền đề: vừa bị từ chối thì còn cooldown")

	if err := fx.svc.Block(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.Unblock(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	row := fx.rowOf(a, b)
	if row == nil || row.Status != model.FriendshipStatusDeclined {
		t.Fatalf("chặn không được xoá/đổi dòng DECLINED (mất cooldown), nhận %+v", row)
	}
	_, err = fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestCooldown, "block -> unblock không được xoá cooldown sau khi bị từ chối")
}

// Cùng họ lỗi với dòng CANCELLED (cooldown thu hồi 1 giờ).
func TestFriendship_Chan_BoChan_KhongXoaCooldownThuHoi(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	id := fx.send(a, b)
	if err := fx.svc.CancelRequest(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.Block(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.Unblock(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	_, err := fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestCooldown, "block -> unblock không được xoá cooldown thu hồi")
}

// M1: hạn mức 20/24h đếm bằng requested_at của chính các dòng; block -> unblock 5 người đã gửi rồi gửi thêm 5
// người mới từng thành công (lách hạn mức). Sau khi sửa dòng còn nguyên nên lời mời thứ 21 vẫn bị chặn.
func TestFriendship_Chan_BoChan_KhongLachHanMuc20(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	first := make([]uuid.UUID, 0, constants.FriendDailyRequestLimit)
	for i := 0; i < constants.FriendDailyRequestLimit; i++ {
		to := fx.student("t")
		fx.send(me, to)
		first = append(first, to)
	}
	extra := fx.student("extra")
	_, err := fx.svc.SendRequest(ctx, me, extra)
	wantErr(t, err, ErrFriendDailyLimit, "tiền đề: lời mời thứ 21 bị chặn")

	for _, to := range first[:5] {
		if err := fx.svc.Block(ctx, me, to); err != nil {
			t.Fatal(err)
		}
		if err := fx.svc.Unblock(ctx, me, to); err != nil {
			t.Fatal(err)
		}
	}
	_, err = fx.svc.SendRequest(ctx, me, extra)
	wantErr(t, err, ErrFriendDailyLimit, "block -> unblock không được hoàn hạn mức 24h")
}

// Block chỉ xoá quan hệ đang hiệu lực: ACCEPTED bị xoá, PENDING chuyển CANCELLED (giữ requested_at để hạn mức
// còn đếm), DECLINED/CANCELLED giữ nguyên.
func TestFriendship_Chan_ChiXoaQuanHeHieuLuc_GiuLichSu(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	friend, pendOut, pendIn, declined := fx.student("f"), fx.student("po"), fx.student("pi"), fx.student("d")
	fx.befriend(me, friend)
	fx.send(me, pendOut)
	fx.send(pendIn, me)
	fx.send(me, declined)
	dec := fx.rowOf(me, declined)
	if _, err := fx.svc.DeclineRequest(ctx, declined, dec.ID); err != nil {
		t.Fatal(err)
	}
	before := fx.rowOf(me, pendOut).RequestedAt
	beforeIn := fx.rowOf(pendIn, me).RequestedAt

	for _, o := range []uuid.UUID{friend, pendOut, pendIn, declined} {
		if err := fx.svc.Block(ctx, me, o); err != nil {
			t.Fatal(err)
		}
	}
	if fx.rowOf(me, friend) != nil {
		t.Error("ACCEPTED phải bị xoá khi chặn")
	}
	if r := fx.rowOf(me, pendOut); r == nil || r.Status != model.FriendshipStatusCancelled || !r.RequestedAt.Equal(before) {
		t.Errorf("PENDING (mình gửi) phải thành CANCELLED và giữ requested_at, nhận %+v", r)
	}
	if r := fx.rowOf(pendIn, me); r == nil || r.Status != model.FriendshipStatusCancelled || !r.RequestedAt.Equal(beforeIn) {
		t.Errorf("PENDING (người kia gửi) phải thành CANCELLED và giữ requested_at, nhận %+v", r)
	}
	if r := fx.rowOf(me, declined); r == nil || r.Status != model.FriendshipStatusDeclined {
		t.Errorf("DECLINED phải được giữ nguyên, nhận %+v", r)
	}
	// Lời mời bị huỷ do chặn không còn hiện ra (CANCELLED vốn ẩn) và không còn tính "đang chờ".
	if sum, _ := fx.svc.Summary(ctx, me); sum.IncomingRequests != 0 || sum.OutgoingRequests != 0 || sum.FriendsCount != 0 {
		t.Errorf("sau khi chặn hết, summary phải bằng 0: %+v", sum)
	}
}

// MINOR 1: Accept và Cancel/Decline chạy đồng thời trên cùng một lời mời. Trước khi sửa hai bên cùng báo thành
// công (Cancel báo "đã huỷ" nhưng trạng thái cuối vẫn ACCEPTED).
func TestFriendship_ChapNhan_DongThoi_ThuHoiHoacTuChoi_KhongCungThanhCong(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(fx *friendFx, sender, receiver, id uuid.UUID) error
		want string
	}{
		{"huỷ", func(fx *friendFx, s, _, id uuid.UUID) error { return fx.svc.CancelRequest(fx.t.Context(), s, id) }, model.FriendshipStatusCancelled},
		{"từ chối", func(fx *friendFx, _, r, id uuid.UUID) error {
			_, err := fx.svc.DeclineRequest(fx.t.Context(), r, id)
			return err
		}, model.FriendshipStatusDeclined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFriendFx(t)
			both := 0
			for i := 0; i < 60; i++ {
				a, b := fx.student("a"), fx.student("b")
				id := fx.send(a, b)
				var accErr, otherErr error
				var wg sync.WaitGroup
				start := make(chan struct{})
				wg.Add(2)
				go func() {
					defer wg.Done()
					<-start
					_, accErr = fx.svc.AcceptRequest(t.Context(), b, id)
				}()
				go func() {
					defer wg.Done()
					<-start
					otherErr = tc.run(fx, a, b, id)
				}()
				close(start)
				wg.Wait()
				final := fx.rowOf(a, b).Status
				switch {
				case accErr == nil && otherErr == nil:
					both++
				case accErr == nil && final != model.FriendshipStatusAccepted:
					t.Fatalf("Accept thành công mà trạng thái cuối là %s", final)
				case otherErr == nil && final != tc.want:
					t.Fatalf("%s thành công mà trạng thái cuối là %s", tc.name, final)
				case accErr != nil && otherErr != nil:
					t.Fatalf("cả hai cùng lỗi: accept=%v %s=%v", accErr, tc.name, otherErr)
				}
			}
			if both != 0 {
				t.Errorf("%d/60 vòng Accept và %s cùng báo thành công (mất cập nhật)", both, tc.name)
			}
		})
	}
}

// MINOR 3: người bị chặn gửi lời mời phải nhận ĐÚNG kết quả như với một người không tồn tại, không được suy ra
// mình bị chặn. Người chặn (biết mình đã chặn) vẫn nhận lỗi rõ ràng.
func TestFriendship_NguoiBiChan_KhongSuyRaMinhBiChan(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	if err := fx.svc.Block(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	_, blockedErr := fx.svc.SendRequest(ctx, b, a)
	_, missingErr := fx.svc.SendRequest(ctx, b, uuid.New())
	wantErr(t, blockedErr, ErrFriendUserNotFound, "người bị chặn gửi cho người chặn")
	if !errors.Is(blockedErr, missingErr) {
		t.Errorf("kết quả phải trùng với người dùng không tồn tại: %v vs %v", blockedErr, missingErr)
	}
	_, err := fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestNotAllowed, "người chặn gửi cho người bị chặn")
	if fx.countRows(a, b) != 0 {
		t.Error("gửi bị từ chối không được để lại dòng")
	}
}

// Vòng 2 NEW-1: tài khoản có vai STUDENT dùng được bạn bè kể cả khi mang vai phụ (PARENT, TEACHER_APPLICANT,
// ORG_OWNER, vai tuỳ chọn); chỉ loại TEACHER đã duyệt và SYSTEM_ADMIN. Trước khi sửa (allowlist cứng) mọi vai
// phụ bị loại, kể cả khỏi Chặn.
func TestFriendship_VaiPhu_VanDungDuoc_ChiLoaiGiaoVienVaAdmin(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	plain := fx.student("plain")
	for _, role := range []string{"PARENT", "TEACHER_APPLICANT", "ORG_OWNER", "CUSTOM_MENTOR"} {
		u := fx.user("sub-"+role, "STUDENT", role)
		if _, err := fx.svc.SendRequest(ctx, u, plain); err != nil {
			t.Errorf("học viên mang vai phụ %s phải kết bạn được: %v", role, err)
		}
	}
	for _, role := range []string{"TEACHER", "SYSTEM_ADMIN"} {
		u := fx.user("adult-"+role, "STUDENT", role)
		_, err := fx.svc.SendRequest(ctx, u, plain)
		wantErr(t, err, ErrFriendRoleNotAllowed, "học viên mang vai "+role)
		_, err = fx.svc.SendRequest(ctx, plain, u)
		wantErr(t, err, ErrFriendUserNotFound, "gửi lời mời cho tài khoản mang vai "+role)
	}
}

// Vòng 2 NEW-1: học viên tự thêm vai PARENT/TEACHER_APPLICANT KHÔNG né được lệnh chặn: chặn người mang vai phụ
// vẫn được, người chặn mang vai phụ vẫn chặn/bỏ chặn/xem danh sách chặn/huỷ kết bạn được. Khoá DM tương ứng được
// kiểm ở conversation_direct_blocked_postgres_test.go (PR nhắn tin).
func TestFriendship_Chan_KhongNeDuocBangVaiPhu(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	s, x := fx.student("s"), fx.student("x")
	fx.befriend(s, x)
	// x tự thêm vai phụ SAU khi đã là bạn (đường tự cấp: POST /auth/me/profiles).
	for _, role := range []string{"PARENT", "TEACHER_APPLICANT"} {
		if err := fx.db.Create(&model.UserSystemRole{UserID: x, SystemRoleID: fx.role(role), Status: model.UserSystemRoleStatusActive}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := fx.svc.Block(ctx, s, x); err != nil {
		t.Fatalf("chặn người mang vai phụ phải được (trước đây 404): %v", err)
	}
	if ok, _ := fx.svc.IsBlockedEitherWay(ctx, s, x); !ok {
		t.Error("lệnh chặn phải có hiệu lực (khoá DM kiểm ở test của PR nhắn tin)")
	}
	// Tài khoản chỉ có vai giáo viên (không STUDENT) cũng chặn được: thao tác an toàn không phụ thuộc allowlist.
	if err := fx.svc.Block(ctx, fx.user("tchr", "TEACHER"), s); err != nil {
		t.Errorf("giáo viên phải chặn được: %v", err)
	}
	// Người chặn cũng tự thêm vai phụ: vẫn xem/bỏ chặn được.
	if err := fx.db.Create(&model.UserSystemRole{UserID: s, SystemRoleID: fx.role("PARENT"), Status: model.UserSystemRoleStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	if list, err := fx.svc.ListBlocks(ctx, s, 1, 20); err != nil || list.TotalCount != 1 {
		t.Errorf("người chặn mang vai phụ phải xem được danh sách chặn: %v %+v", err, list)
	}
	if err := fx.svc.Unblock(ctx, s, x); err != nil {
		t.Errorf("người chặn mang vai phụ phải bỏ chặn được: %v", err)
	}
	// Huỷ kết bạn của tài khoản mang vai phụ.
	y := fx.student("y")
	fx.befriend(s, y)
	if err := fx.svc.Unfriend(ctx, s, y); err != nil {
		t.Errorf("huỷ kết bạn của tài khoản mang vai phụ phải được: %v", err)
	}
}

// Vòng 2 NEW-6: DELETE /friends/requests/:id của người bị từ chối và của người bị chặn phải cho CÙNG một kết
// quả (không suy ra mình bị từ chối hay bị chặn).
func TestFriendship_Huy_BiTuChoiVaBiChan_CungKetQua(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b, c, d := fx.student("a"), fx.student("b"), fx.student("c"), fx.student("d")
	declinedID := fx.send(a, b)
	if _, err := fx.svc.DeclineRequest(ctx, b, declinedID); err != nil {
		t.Fatal(err)
	}
	blockedID := fx.send(c, d)
	if err := fx.svc.Block(ctx, d, c); err != nil {
		t.Fatal(err)
	}
	errDeclined := fx.svc.CancelRequest(ctx, a, declinedID)
	errBlocked := fx.svc.CancelRequest(ctx, c, blockedID)
	wantErr(t, errDeclined, ErrFriendRequestNotFound, "huỷ lời mời đã bị từ chối")
	wantErr(t, errBlocked, ErrFriendRequestNotFound, "huỷ lời mời của người bị chặn")
}

// Vòng 2 mục 7: cooldown trả kèm thời gian còn phải chờ (retry_after) và vẫn khớp ErrFriendRequestCooldown.
func TestFriendship_Cooldown_KemThoiGianConLai(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	id := fx.send(a, b)
	if _, err := fx.svc.DeclineRequest(ctx, b, id); err != nil {
		t.Fatal(err)
	}
	_, err := fx.svc.SendRequest(ctx, a, b)
	var cd *FriendCooldownError
	if !errors.As(err, &cd) || !errors.Is(err, ErrFriendRequestCooldown) {
		t.Fatalf("muốn FriendCooldownError khớp ErrFriendRequestCooldown, nhận %v", err)
	}
	if cd.RetryAfter <= 6*24*time.Hour || cd.RetryAfter > constants.FriendDeclineCooldown {
		t.Errorf("retry_after sau từ chối vừa xong phải gần 7 ngày, nhận %v", cd.RetryAfter)
	}
}

// MINOR 5 / vòng 2 NEW-5: che user_name dạng email-prefix chỉ khi tài khoản là Google VÀ trùng prefix email, và
// nhất quán ở MỌI response bạn bè (gửi lời mời, danh sách lời mời, danh sách chặn, danh sách bạn, tìm kiếm).
func TestFriendship_UserNameEmailPrefix_CheNhatQuan_ChiVoiGoogle(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	g := fx.student("g") // tài khoản Google: user_name = phần trước @
	if err := fx.db.Model(&model.User{}).Where("id = ?", g).
		Updates(map[string]any{"email": "zork.peter99@gmail.test", "user_name": "zork.peter99", "full_name": "Quill Hansen"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&model.UserOAuthProvider{UserID: g, Provider: "google", ProviderUserID: "gid-" + g.String()}).Error; err != nil {
		t.Fatal(err)
	}
	self := fx.student("self") // tự đặt user_name trùng prefix email nhưng KHÔNG phải Google: không che
	if err := fx.db.Model(&model.User{}).Where("id = ?", self).
		Updates(map[string]any{"email": "tutu.chon@mail.test", "user_name": "tutu.chon", "full_name": "Tu Chon"}).Error; err != nil {
		t.Fatal(err)
	}

	if res, _ := fx.svc.Search(ctx, me, "zork.peter", 20); len(res.Users) != 0 {
		t.Errorf("tìm theo phần trước @ của tài khoản Google không được ra người dùng: %+v", res.Users)
	}
	if res, _ := fx.svc.Search(ctx, me, "tutu.chon", 20); len(res.Users) != 1 || res.Users[0].UserName != "tutu.chon" {
		t.Errorf("người tự đặt user_name không phải Google không bị che nhầm: %+v", res.Users)
	}
	out, err := fx.svc.SendRequest(ctx, me, g)
	if err != nil {
		t.Fatal(err)
	}
	if out.Result.User.UserName != "" {
		t.Errorf("SendRequest lộ user_name email-prefix: %q", out.Result.User.UserName)
	}
	if reqs, _ := fx.svc.ListRequests(ctx, me, "outgoing", 1, 20); len(reqs.Requests) != 1 || reqs.Requests[0].User.UserName != "" {
		t.Errorf("danh sách lời mời đi lộ user_name: %+v", reqs.Requests)
	}
	fx.befriend(me, fx.student("other"))
	if err := fx.svc.Block(ctx, me, g); err != nil {
		t.Fatal(err)
	}
	if bl, _ := fx.svc.ListBlocks(ctx, me, 1, 20); len(bl.Blocks) != 1 || bl.Blocks[0].User.UserName != "" {
		t.Errorf("danh sách chặn lộ user_name: %+v", bl.Blocks)
	}
	fx.befriend(g, fx.student("friend2"))
	if fl, _ := fx.svc.ListFriends(ctx, g, "", 1, 20); len(fl.Friends) != 1 || fl.Friends[0].User.UserName == "" {
		t.Errorf("danh sách bạn của người khác vẫn hiện user_name thường: %+v", fl.Friends)
	}
}
// Quyết định chủ dự án (sau review): huỷ lời mời KHÔNG tạo cooldown ở chiều ngược lại, và cooldown dài chỉ bật
// khi bị TỪ CHỐI. Với chính người huỷ chỉ còn cooldown rất ngắn (chống gửi-huỷ-gửi quấy rối vì dòng bị tái sử dụng
// nên không tính thêm vào hạn mức), nên sau vài phút gửi lại được. Trước khi sửa cooldown này là 1 giờ.
func TestFriendship_Huy_KhongCooldownChieuNguoc_VaNguoiHuyChiChoNgan(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	id := fx.send(a, b)
	if err := fx.svc.CancelRequest(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	// Chiều ngược lại (người nhận cũ chủ động gửi cho người đã huỷ): không bị cooldown.
	if _, err := fx.svc.SendRequest(ctx, b, a); err != nil {
		t.Errorf("sau khi a huỷ, b vẫn phải gửi được cho a ngay: %v", err)
	}

	// Chiều người huỷ: chỉ cooldown ngắn, hết sau vài phút (không phải 1 giờ).
	c, d := fx.student("c"), fx.student("d")
	id2 := fx.send(c, d)
	if err := fx.svc.CancelRequest(ctx, c, id2); err != nil {
		t.Fatal(err)
	}
	_, err := fx.svc.SendRequest(ctx, c, d)
	wantErr(t, err, ErrFriendRequestCooldown, "gửi lại ngay sau khi tự huỷ vẫn bị cooldown ngắn (chống gửi-huỷ-gửi)")
	fx.age(c, d, 6*time.Minute)
	if _, err := fx.svc.SendRequest(ctx, c, d); err != nil {
		t.Errorf("sau 6 phút người huỷ phải gửi lại được: %v", err)
	}
}