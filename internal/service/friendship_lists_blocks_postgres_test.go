package service

// Test Postgres THẬT: vai trò được phép, danh sách, tìm kiếm, relationship và chặn (phase 01, plan 260930).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// Mọi API Bạn bè chỉ dành cho HỌC VIÊN (Q1): phụ huynh, giáo viên, admin, người vừa là học viên vừa là giáo
// viên, tài khoản không vai trò và tài khoản bị khoá đều nhận ErrFriendRoleNotAllowed ở MỌI thao tác.
// Bỏ requireStudent khỏi một thao tác thì dòng tương ứng ĐỎ.
func TestFriendship_ChiHocVien_MoiThaoTac(t *testing.T) {
	fx := newFriendFx(t)
	student := fx.student("ok")
	locked := fx.student("locked")
	fx.deactivate(locked)
	callers := map[string]uuid.UUID{
		"phụ huynh":                  fx.user("parent", "PARENT"),
		"giáo viên":                  fx.user("teacher", "TEACHER"),
		"admin":                      fx.user("admin", "SYSTEM_ADMIN"),
		"vừa học viên vừa giáo viên": fx.user("both", "STUDENT", "TEACHER"),
		"không vai trò":              fx.user("norole"),
		"học viên bị khoá":           locked,
	}
	ops := map[string]func(ctx context.Context, me uuid.UUID) error{
		"SendRequest": func(ctx context.Context, me uuid.UUID) error {
			_, err := fx.svc.SendRequest(ctx, me, student)
			return err
		},
		"AcceptRequest": func(ctx context.Context, me uuid.UUID) error {
			_, err := fx.svc.AcceptRequest(ctx, me, uuid.New())
			return err
		},
		"DeclineRequest": func(ctx context.Context, me uuid.UUID) error {
			_, err := fx.svc.DeclineRequest(ctx, me, uuid.New())
			return err
		},
		"CancelRequest": func(ctx context.Context, me uuid.UUID) error { return fx.svc.CancelRequest(ctx, me, uuid.New()) },
		"Unfriend":      func(ctx context.Context, me uuid.UUID) error { return fx.svc.Unfriend(ctx, me, student) },
		"ListFriends": func(ctx context.Context, me uuid.UUID) error {
			_, err := fx.svc.ListFriends(ctx, me, "", 1, 20)
			return err
		},
		"Summary": func(ctx context.Context, me uuid.UUID) error { _, err := fx.svc.Summary(ctx, me); return err },
		"ListRequests": func(ctx context.Context, me uuid.UUID) error {
			_, err := fx.svc.ListRequests(ctx, me, "", 1, 20)
			return err
		},
		"Search": func(ctx context.Context, me uuid.UUID) error { _, err := fx.svc.Search(ctx, me, "abc", 20); return err },
		"Relationship": func(ctx context.Context, me uuid.UUID) error {
			_, err := fx.svc.Relationship(ctx, me, student)
			return err
		},
		"ListBlocks": func(ctx context.Context, me uuid.UUID) error { _, err := fx.svc.ListBlocks(ctx, me, 1, 20); return err },
		"Block":      func(ctx context.Context, me uuid.UUID) error { return fx.svc.Block(ctx, me, student) },
		"Unblock":    func(ctx context.Context, me uuid.UUID) error { return fx.svc.Unblock(ctx, me, student) },
	}
	for who, me := range callers {
		for name, op := range ops {
			wantErr(t, op(t.Context(), me), ErrFriendRoleNotAllowed, who+" gọi "+name)
		}
	}
}

// Đích phải là học viên đang hoạt động và không ẩn hồ sơ; mọi lý do khác đều là "không tìm thấy", không lộ
// việc tài khoản có tồn tại hay mang vai trò gì.
func TestFriendship_DichKhongHopLe_KhongTimThay(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	hidden := fx.student("hidden")
	fx.setVisibility(hidden, "hidden")
	locked := fx.student("locked")
	fx.deactivate(locked)
	targets := map[string]uuid.UUID{
		"phụ huynh":            fx.user("p", "PARENT"),
		"giáo viên":            fx.user("t", "TEACHER"),
		"admin":                fx.user("a", "SYSTEM_ADMIN"),
		"học viên + giáo viên": fx.user("b", "STUDENT", "TEACHER"),
		"ẩn hồ sơ":             hidden,
		"bị khoá":              locked,
		"không tồn tại":        uuid.New(),
	}
	for who, id := range targets {
		_, err := fx.svc.SendRequest(ctx, me, id)
		wantErr(t, err, ErrFriendUserNotFound, "gửi lời mời tới "+who)
		if n := fx.countRows(me, id); n != 0 {
			t.Errorf("gửi tới %s không được tạo dòng", who)
		}
	}
	// Ẩn hồ sơ chỉ chặn việc bị tìm/mời mới; quan hệ đã có vẫn hiển thị đúng.
	rel, err := fx.svc.Relationship(ctx, me, hidden)
	if err != nil || rel.Status != dto.RelationNone {
		t.Errorf("relationship tới người ẩn hồ sơ: %v %+v", err, rel)
	}
	_, err = fx.svc.Relationship(ctx, me, targets["giáo viên"])
	wantErr(t, err, ErrFriendUserNotFound, "relationship tới giáo viên")
}

func TestFriendship_Relationship_MoiTrangThai(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	check := func(other uuid.UUID, want string, wantReq bool) {
		t.Helper()
		got, err := fx.svc.Relationship(ctx, me, other)
		if err != nil {
			t.Fatalf("Relationship: %v", err)
		}
		if got.Status != want || (got.RequestID != nil) != wantReq {
			t.Errorf("muốn %s (request_id=%v), nhận %+v", want, wantReq, got)
		}
	}
	check(me, dto.RelationSelf, false)

	none := fx.student("none")
	check(none, dto.RelationNone, false)

	friend := fx.student("friend")
	fx.befriend(me, friend)
	check(friend, dto.RelationFriends, false)

	out := fx.student("out")
	fx.send(me, out)
	check(out, dto.RelationPendingOut, true)

	in := fx.student("in")
	fx.send(in, me)
	check(in, dto.RelationPendingIn, true)

	// DECLINED không lộ ra: người gửi chỉ thấy NONE.
	declined := fx.student("declined")
	id := fx.send(me, declined)
	if _, err := fx.svc.DeclineRequest(ctx, declined, id); err != nil {
		t.Fatal(err)
	}
	check(declined, dto.RelationNone, false)

	blockedByMe := fx.student("bbm")
	if err := fx.svc.Block(ctx, me, blockedByMe); err != nil {
		t.Fatal(err)
	}
	check(blockedByMe, dto.RelationBlockedByMe, false)

	// Người kia chặn mình: mình chỉ thấy NONE (không lộ việc bị chặn).
	blocker := fx.student("blocker")
	if err := fx.svc.Block(ctx, blocker, me); err != nil {
		t.Fatal(err)
	}
	check(blocker, dto.RelationNone, false)
}

func TestFriendship_DanhSachBan_PhanTrangTimKiemVaChiAccepted(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	for i := 0; i < 5; i++ {
		fx.befriend(me, fx.namedStudent(fmt.Sprintf("bando%d", i), fmt.Sprintf("Ban Do %d", i)))
	}
	fx.befriend(fx.namedStudent("khac", "Nguoi Khac"), me) // chiều ngược vẫn là bạn của me
	fx.send(me, fx.student("pending"))                     // không phải bạn
	locked := fx.student("locked")
	fx.befriend(me, locked)
	fx.deactivate(locked) // bạn đã bị khoá thì không hiển thị

	res, err := fx.svc.ListFriends(ctx, me, "", 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalCount != 6 || len(res.Friends) != 4 || res.Page != 1 || res.Limit != 4 {
		t.Fatalf("muốn total=6, 4 dòng trang 1: %+v", res)
	}
	page2, _ := fx.svc.ListFriends(ctx, me, "", 2, 4)
	if len(page2.Friends) != 2 {
		t.Errorf("trang 2 muốn 2 dòng, nhận %d", len(page2.Friends))
	}
	seen := map[uuid.UUID]bool{}
	for _, f := range append(res.Friends, page2.Friends...) {
		if seen[f.User.UserID] {
			t.Errorf("trùng người giữa các trang: %v", f.User.UserID)
		}
		seen[f.User.UserID] = true
	}

	byName, _ := fx.svc.ListFriends(ctx, me, "khac", 1, 20)
	if byName.TotalCount != 1 || byName.Friends[0].User.UserName != "khac" {
		t.Errorf("lọc theo tên: %+v", byName)
	}
	// Giới hạn limit tối đa 100, sai thì về mặc định.
	big, _ := fx.svc.ListFriends(ctx, me, "", 0, 9999)
	if big.Limit != constants.FriendPageMaxLimit || big.Page != 1 {
		t.Errorf("chuẩn hoá page/limit: page=%d limit=%d", big.Page, big.Limit)
	}
	def, _ := fx.svc.ListFriends(ctx, me, "", 1, 0)
	if def.Limit != constants.FriendPageDefaultLimit {
		t.Errorf("limit mặc định muốn %d, nhận %d", constants.FriendPageDefaultLimit, def.Limit)
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), "@") || strings.Contains(strings.ToLower(string(raw)), "email") {
		t.Errorf("danh sách bạn không được lộ email: %s", raw)
	}
}

func TestFriendship_DanhSachLoiMoi_VaSummary(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	in1, in2, out1 := fx.student("in1"), fx.student("in2"), fx.student("out1")
	fx.send(in1, me)
	fx.send(in2, me)
	fx.send(me, out1)
	fx.befriend(me, fx.student("f"))
	declined := fx.student("d")
	id := fx.send(declined, me)
	if _, err := fx.svc.DeclineRequest(ctx, me, id); err != nil {
		t.Fatal(err)
	}

	incoming, err := fx.svc.ListRequests(ctx, me, "", 1, 20) // mặc định = incoming
	if err != nil || incoming.TotalCount != 2 {
		t.Fatalf("incoming mặc định muốn 2, nhận %+v (%v)", incoming, err)
	}
	for _, r := range incoming.Requests {
		if r.Direction != "incoming" || (r.User.UserID != in1 && r.User.UserID != in2) {
			t.Errorf("dòng incoming sai: %+v", r)
		}
	}
	outgoing, _ := fx.svc.ListRequests(ctx, me, "outgoing", 1, 20)
	if outgoing.TotalCount != 1 || outgoing.Requests[0].User.UserID != out1 || outgoing.Requests[0].Direction != "outgoing" {
		t.Errorf("outgoing sai: %+v", outgoing)
	}
	_, err = fx.svc.ListRequests(ctx, me, "sideways", 1, 20)
	wantErr(t, err, ErrFriendInvalidDirection, "direction lạ")

	sum, _ := fx.svc.Summary(ctx, me)
	if sum.FriendsCount != 1 || sum.IncomingRequests != 2 || sum.OutgoingRequests != 1 {
		t.Errorf("summary sai (DECLINED không được tính vào đang chờ): %+v", sum)
	}
}

func TestFriendship_TimKiem(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.namedStudent("toi-tim-kiem", "Nguoi Tim")

	_, err := fx.svc.Search(ctx, me, "ab", 20)
	wantErr(t, err, ErrFriendQueryTooShort, "từ khoá 2 ký tự")
	_, err = fx.svc.Search(ctx, me, "  a  ", 20)
	wantErr(t, err, ErrFriendQueryTooShort, "từ khoá chỉ có khoảng trắng")
	if _, err := fx.svc.Search(ctx, me, "Ngô", 20); err != nil {
		t.Errorf("3 ký tự Unicode (Ngô) phải đủ điều kiện: %v", err)
	}

	match := fx.namedStudent("hoc-sinh-mai", "Nguyen Thi Mai")
	friend := fx.namedStudent("mai-ban", "Mai Ban Than")
	fx.befriend(me, friend)
	pendOut := fx.namedStudent("mai-out", "Mai Out")
	outReq := fx.send(me, pendOut)
	pendIn := fx.namedStudent("mai-in", "Mai In")
	inReq := fx.send(pendIn, me)
	declined := fx.namedStudent("mai-declined", "Mai Declined")
	if _, err := fx.svc.DeclineRequest(ctx, declined, fx.send(me, declined)); err != nil {
		t.Fatal(err)
	}
	// Không được xuất hiện:
	fx.namedStudent("mai-khoa", "Mai Bi Khoa")
	hidden := fx.namedStudent("mai-an", "Mai An Ho So")
	fx.setVisibility(hidden, "hidden")
	blocked := fx.namedStudent("mai-chan", "Mai Bi Chan")
	if err := fx.svc.Block(ctx, me, blocked); err != nil {
		t.Fatal(err)
	}
	blocker := fx.namedStudent("mai-chan-toi", "Mai Chan Toi")
	if err := fx.svc.Block(ctx, blocker, me); err != nil {
		t.Fatal(err)
	}
	fx.user("mai-gv", "TEACHER")
	if err := fx.db.Model(&model.User{}).Where("user_name LIKE ?", "mai-gv%").Update("user_name", "mai-giao-vien").Error; err != nil {
		t.Fatal(err)
	}
	fx.user("mai-ph", "PARENT")
	if err := fx.db.Model(&model.User{}).Where("user_name LIKE ?", "mai-ph%").Update("user_name", "mai-phu-huynh").Error; err != nil {
		t.Fatal(err)
	}
	lockedID := fx.namedStudent("mai-locked", "Mai Locked")
	fx.deactivate(lockedID)
	if err := fx.db.Model(&model.User{}).Where("user_name = ?", "mai-khoa").Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}

	res, err := fx.svc.Search(ctx, me, "mai", 20)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]string{}
	for _, u := range res.Users {
		got[u.UserID] = u.Relationship
		// request_id chỉ có khi đang có lời mời chờ (PENDING_OUT/PENDING_IN) và đúng id lời mời.
		wantReq := map[uuid.UUID]uuid.UUID{pendOut: outReq, pendIn: inReq}[u.UserID]
		switch {
		case wantReq != uuid.Nil && (u.RequestID == nil || *u.RequestID != wantReq):
			t.Errorf("user %v (%s): muốn request_id %v, nhận %v", u.UserID, u.Relationship, wantReq, u.RequestID)
		case wantReq == uuid.Nil && u.RequestID != nil:
			t.Errorf("user %v (%s): không có lời mời chờ nhưng có request_id %v", u.UserID, u.Relationship, *u.RequestID)
		}
	}
	want := map[uuid.UUID]string{
		match: dto.RelationNone, friend: dto.RelationFriends, pendOut: dto.RelationPendingOut,
		pendIn: dto.RelationPendingIn, declined: dto.RelationNone,
	}
	if len(got) != len(want) {
		t.Fatalf("muốn %d kết quả %v, nhận %d: %v", len(want), want, len(got), got)
	}
	for id, rel := range want {
		if got[id] != rel {
			t.Errorf("user %v: muốn relationship %s, nhận %q", id, rel, got[id])
		}
	}
	if _, self := got[me]; self {
		t.Error("kết quả không được chứa chính mình")
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(strings.ToLower(string(raw)), "email") || strings.Contains(string(raw), "@") || strings.Contains(string(raw), "phone") {
		t.Errorf("kết quả tìm kiếm không được lộ email/điện thoại: %s", raw)
	}
	if !strings.Contains(string(raw), `"relationship"`) || !strings.Contains(string(raw), `"user_id"`) {
		t.Errorf("JSON phải phẳng {user_id,user_name,...,relationship}: %s", raw)
	}
}

// Ký tự đại diện của LIKE phải được thoát: "50%" chỉ tìm chuỗi "50%", không khớp mọi tên bắt đầu bằng 50.
func TestFriendship_TimKiem_ThoatKyTuDaiDien(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	exact := fx.namedStudent("gia-50%-off", "Nam")
	fx.namedStudent("so-500-abc", "Bao")
	fx.namedStudent("a_b_c", "Cuong")
	fx.namedStudent("axbxc", "Dung")

	res, _ := fx.svc.Search(ctx, me, "50%", 20)
	if len(res.Users) != 1 || res.Users[0].UserID != exact {
		t.Errorf(`"50%%" chỉ được khớp đúng 1 người có ký tự %%, nhận %+v`, res.Users)
	}
	res, _ = fx.svc.Search(ctx, me, "a_b", 20)
	if len(res.Users) != 1 || res.Users[0].UserName != "a_b_c" {
		t.Errorf(`"a_b" phải khớp nghĩa đen (không phải 1 ký tự bất kỳ), nhận %+v`, res.Users)
	}
}

func TestFriendship_TimKiem_ToiDa20KetQua(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	me := fx.student("me")
	for i := 0; i < constants.FriendSearchMaxResults+5; i++ {
		fx.namedStudent(fmt.Sprintf("lop-mot-%02d", i), fmt.Sprintf("Hoc Sinh %02d", i))
	}
	for _, tc := range []struct{ asked, want int }{{0, 20}, {5, 5}, {100, 20}, {-3, 20}} {
		res, err := fx.svc.Search(ctx, me, "lop-mot", tc.asked)
		if err != nil || len(res.Users) != tc.want {
			t.Errorf("limit=%d: muốn %d kết quả, nhận %d (%v)", tc.asked, tc.want, len(res.Users), err)
		}
	}
}

func TestFriendship_Chan_XoaBanVaLoiMoi_ChanHaiChieu(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b := fx.student("a"), fx.student("b")
	fx.befriend(a, b)

	if err := fx.svc.Block(ctx, a, b); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if fx.rowOf(a, b) != nil {
		t.Error("chặn phải xoá tình bạn")
	}
	if blocked, _ := fx.svc.IsBlockedEitherWay(ctx, a, b); !blocked {
		t.Error("IsBlockedEitherWay(a,b)")
	}
	if blocked, _ := fx.svc.IsBlockedEitherWay(ctx, b, a); !blocked {
		t.Error("IsBlockedEitherWay(b,a) phải đối xứng")
	}
	if ok, _ := fx.svc.AreFriends(ctx, a, b); ok {
		t.Error("sau khi chặn không còn là bạn")
	}

	// Cả hai chiều đều không gửi được. Người bị chặn nhận kết quả y hệt người dùng không tồn tại (không đoán
	// ra mình bị chặn); người chặn biết mình đã chặn nên nhận lỗi rõ ràng.
	_, err := fx.svc.SendRequest(ctx, b, a)
	wantErr(t, err, ErrFriendUserNotFound, "người bị chặn gửi cho người chặn")
	_, err = fx.svc.SendRequest(ctx, a, b)
	wantErr(t, err, ErrFriendRequestNotAllowed, "người chặn gửi cho người bị chặn")
	if fx.rowOf(a, b) != nil {
		t.Error("gửi bị từ chối không được để lại dòng")
	}

	// Chặn idempotent, và có trong danh sách chặn.
	if err := fx.svc.Block(ctx, a, b); err != nil {
		t.Errorf("chặn lần hai phải thành công (idempotent): %v", err)
	}
	list, _ := fx.svc.ListBlocks(ctx, a, 1, 20)
	if list.TotalCount != 1 || list.Blocks[0].User.UserID != b {
		t.Errorf("danh sách chặn sai: %+v", list)
	}
	if other, _ := fx.svc.ListBlocks(ctx, b, 1, 20); other.TotalCount != 0 {
		t.Error("người bị chặn không có mục nào trong danh sách chặn của mình")
	}

	// Bỏ chặn không khôi phục tình bạn, nhưng mở lại khả năng gửi lời mời.
	if err := fx.svc.Unblock(ctx, a, b); err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	if blocked, _ := fx.svc.IsBlockedEitherWay(ctx, a, b); blocked {
		t.Error("đã bỏ chặn mà vẫn báo chặn")
	}
	if ok, _ := fx.svc.AreFriends(ctx, a, b); ok {
		t.Error("bỏ chặn không được khôi phục tình bạn")
	}
	if err := fx.svc.Unblock(ctx, a, b); err != nil {
		t.Errorf("bỏ chặn người chưa chặn phải idempotent: %v", err)
	}
	if _, err := fx.svc.SendRequest(ctx, b, a); err != nil {
		t.Errorf("sau khi bỏ chặn phải gửi được: %v", err)
	}
}

func TestFriendship_Chan_XoaLoiMoiDangCho_Va_TuChanMinh(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a, b, c := fx.student("a"), fx.student("b"), fx.student("c")
	fx.send(a, b)
	fx.send(c, a)

	if err := fx.svc.Block(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	// Lời mời đang chờ không bị xoá mà chuyển CANCELLED (giữ lịch sử cho hạn mức/cooldown); không còn hiệu lực.
	if r := fx.rowOf(a, b); r == nil || r.Status != model.FriendshipStatusCancelled {
		t.Errorf("chặn phải huỷ lời mời đang chờ (chiều a->b), nhận %+v", r)
	}
	if err := fx.svc.Block(ctx, b, c); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.Block(ctx, a, c); err != nil {
		t.Fatal(err)
	}
	if r := fx.rowOf(a, c); r == nil || r.Status != model.FriendshipStatusCancelled {
		t.Errorf("chặn phải huỷ lời mời đang chờ (chiều c->a), nhận %+v", r)
	}
	wantErr(t, fx.svc.Block(ctx, a, a), ErrFriendSelfRequest, "tự chặn mình")
	wantErr(t, fx.svc.Block(ctx, a, fx.user("gv", "TEACHER")), ErrFriendUserNotFound, "chặn giáo viên")
	wantErr(t, fx.svc.Block(ctx, a, uuid.New()), ErrFriendUserNotFound, "chặn id không tồn tại")
}

// Chặn không được làm lộ người chặn qua tìm kiếm: cả hai phía đều không thấy nhau.
func TestFriendship_Chan_AnKhoiTimKiemHaiPhia(t *testing.T) {
	fx := newFriendFx(t)
	ctx := t.Context()
	a := fx.namedStudent("zeta-aaa", "Zeta A")
	b := fx.namedStudent("zeta-bbb", "Zeta B")
	if err := fx.svc.Block(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	for who, from := range map[string]uuid.UUID{"người chặn": a, "người bị chặn": b} {
		res, _ := fx.svc.Search(ctx, from, "zeta", 20)
		if len(res.Users) != 0 {
			t.Errorf("%s vẫn tìm thấy đối phương: %+v", who, res.Users)
		}
	}
}
