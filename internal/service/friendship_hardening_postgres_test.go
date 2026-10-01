package service

// Test Postgres THẬT cho các lỗ do review đối kháng #101 chỉ ra (plans/reports/review-261001-lane-gf-be.md):
// M1 (chặn rồi bỏ chặn xoá cooldown và hạn mức), MINOR 1 (huỷ/từ chối không khoá), 3 (người bị chặn đoán ra
// mình bị chặn), 4 (lọc vai trò denylist), 5 (tìm kiếm lộ phần trước @ của email).

import (
	"errors"
	"sync"
	"testing"

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

// MINOR 4: lọc vai trò là allowlist. Học viên mang thêm BẤT KỲ vai trò hệ thống nào khác (kể cả vai trò tự tạo
// sau này) đều bị loại, cả khi gọi API lẫn khi bị tìm kiếm/được mời.
func TestFriendship_VaiTro_Allowlist_VaiTroTuyChon_BiLoai(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	plain := fx.student("plain")
	mentor := fx.user("mentor", "STUDENT", "CUSTOM_MENTOR")
	fx.db.Model(&model.User{}).Where("id = ?", mentor).Update("user_name", "zzmentor")

	_, err := fx.svc.SendRequest(ctx, mentor, plain)
	wantErr(t, err, ErrFriendRoleNotAllowed, "học viên kèm vai trò tự tạo gọi API")
	_, err = fx.svc.SendRequest(ctx, plain, mentor)
	wantErr(t, err, ErrFriendUserNotFound, "gửi lời mời cho học viên kèm vai trò tự tạo")
	if res, _ := fx.svc.Search(ctx, plain, "zzmentor", 20); len(res.Users) != 0 {
		t.Errorf("học viên kèm vai trò tự tạo không được hiện trong tìm kiếm: %+v", res.Users)
	}
	if _, err := fx.svc.SendRequest(ctx, plain, fx.student("ok")); err != nil {
		t.Errorf("học viên thuần phải vẫn dùng được: %v", err)
	}
}

// MINOR 5: tài khoản Google có user_name = phần trước @ của email. Không được khớp và không được trả giá trị
// đó trong tìm kiếm (lộ một phần email); khớp theo họ tên vẫn được nhưng user_name trả rỗng.
func TestFriendship_TimKiem_KhongLoPhanTruocEmail(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	g := fx.student("g")
	if err := fx.db.Model(&model.User{}).Where("id = ?", g).
		Updates(map[string]any{"email": "zork.peter99@gmail.test", "user_name": "zork.peter99", "full_name": "Quill Hansen"}).Error; err != nil {
		t.Fatal(err)
	}
	normal := fx.namedStudent("zork-normal", "Zork Normal")

	res, _ := fx.svc.Search(ctx, me, "zork.peter", 20)
	if len(res.Users) != 0 {
		t.Errorf("tìm theo phần trước @ của email không được ra người dùng: %+v", res.Users)
	}
	res, _ = fx.svc.Search(ctx, me, "Quill", 20)
	if len(res.Users) != 1 || res.Users[0].UserID != g {
		t.Fatalf("tìm theo họ tên phải ra đúng người: %+v", res.Users)
	}
	if res.Users[0].UserName != "" {
		t.Errorf("user_name sinh từ email không được trả về: %q", res.Users[0].UserName)
	}
	res, _ = fx.svc.Search(ctx, me, "zork-normal", 20)
	if len(res.Users) != 1 || res.Users[0].UserID != normal || res.Users[0].UserName != "zork-normal" {
		t.Errorf("tài khoản thường vẫn tìm và trả user_name như cũ: %+v", res.Users)
	}
}
