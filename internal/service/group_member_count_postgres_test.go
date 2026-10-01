package service

// Test Postgres THẬT cho lỗi review đối kháng #102 M3 (plans/reports/review-261001-lane-gf-be.md): cùng một
// người có dòng LEFT cũ vào nhóm đồng thời làm member_count trôi (+2 cho 1 thành viên) vì activateMember đọc
// dòng cũ không khoá. Nay chỉ giữ chỗ được giữ lại khi chính request này chuyển dòng sang ACTIVE.

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// memberCountDrift trả (member_count đã lưu, số thành viên ACTIVE thật) của nhóm.
func (fx *groupFx) memberCountDrift(gid uuid.UUID) (stored int, active int64) {
	fx.t.Helper()
	var g model.Group
	if err := fx.db.First(&g, "id = ?", gid).Error; err != nil {
		fx.t.Fatal(err)
	}
	fx.db.Model(&model.GroupMember{}).Where("group_id = ? AND status = ?", gid, model.GroupMemberActive).Count(&active)
	return g.MemberCount, active
}

// Mỗi vòng: một người có dòng LEFT cũ, hai request vào nhóm cùng lúc. Đúng một request thắng, request kia nhận
// ErrGroupAlreadyMember, và member_count luôn bằng số ACTIVE thật. Trước khi sửa mỗi vòng trôi +1 (2 lần giữ chỗ).
func TestGroupMember_DongThoi_CungMotNguoiCoDongLEFT_KhongTroiMemberCount(t *testing.T) {
	fx := newGroupFx(t)
	admin := fx.adminOwner()
	g := fx.group(admin, model.GroupPrivacyPublic, 500)
	const rounds = 40
	for i := 0; i < rounds; i++ {
		u := guardUser(t, fx.db, "left")
		fx.member(g.ID, u, model.GroupRoleAdmin, model.GroupMemberLeft)
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, 2)
		wg.Add(2)
		go func() { // đường vào công khai
			defer wg.Done()
			<-start
			_, errs[0] = fx.svc.JoinGroup(t.Context(), u, g.ID, nil)
		}()
		go func() { // đường mời bởi chủ nhóm
			defer wg.Done()
			<-start
			res, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{u})
			if err == nil && len(res.Invited) == 0 {
				err = ErrGroupAlreadyMember
			}
			errs[1] = err
		}()
		close(start)
		wg.Wait()
		wins := 0
		for _, err := range errs {
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrGroupAlreadyMember):
			default:
				t.Fatalf("vòng %d: lỗi ngoài dự kiến: %v", i, err)
			}
		}
		if wins != 1 {
			t.Fatalf("vòng %d: muốn đúng 1 request chuyển sang ACTIVE, nhận %d", i, wins)
		}
		if stored, active := fx.memberCountDrift(g.ID); int64(stored) != active {
			t.Fatalf("vòng %d: member_count trôi: lưu %d nhưng ACTIVE thật %d", i, stored, active)
		}
	}
}

// Cùng người vào hai lần song song (join+join): cũng không trôi, và dòng cũ được kích hoạt lại với vai trò MEMBER.
func TestGroupMember_DongThoi_JoinHaiLan_KhongTroiMemberCount(t *testing.T) {
	fx := newGroupFx(t)
	owner := guardUser(t, fx.db, "owner")
	g := fx.group(owner, model.GroupPrivacyPublic, 500)
	for i := 0; i < 40; i++ {
		u := guardUser(t, fx.db, "left2")
		fx.member(g.ID, u, model.GroupRoleAdmin, model.GroupMemberLeft)
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, 2)
		for k := range errs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errs[k] = fx.svc.JoinGroup(t.Context(), u, g.ID, nil)
			}()
		}
		close(start)
		wg.Wait()
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatalf("vòng %d: đúng một request phải thắng, nhận %v / %v", i, errs[0], errs[1])
		}
		if stored, active := fx.memberCountDrift(g.ID); int64(stored) != active {
			t.Fatalf("vòng %d: member_count trôi: lưu %d nhưng ACTIVE thật %d", i, stored, active)
		}
		if r := fx.row(g.ID, u); r == nil || r.Role != model.GroupRoleMember {
			t.Fatalf("vòng %d: vai trò phải được đặt lại MEMBER: %+v", i, r)
		}
	}
}