package service

// Lane web cần hiển thị tên thật của thành viên nhóm: item danh sách thành viên có full_name và avatar_url
// (trước đây chỉ có user_name).

import (
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestListMembers_CoFullNameVaAvatar(t *testing.T) {
	fx := newGroupFx(t)
	owner := guardUser(t, fx.db, "owner")
	g := fx.group(owner, model.GroupPrivacyPublic, 10)
	m := guardUser(t, fx.db, "named")
	fx.member(g.ID, m, model.GroupRoleMember, model.GroupMemberActive)
	if err := fx.db.Model(&model.User{}).Where("id = ?", m).
		Updates(map[string]any{"full_name": "Nguyễn Thị Hoa", "avatar_url": "https://img.test/hoa.png"}).Error; err != nil {
		t.Fatal(err)
	}

	res, err := fx.svc.ListMembers(t.Context(), owner, g.ID, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	var got bool
	for _, it := range res.Members {
		if it.UserID != m {
			continue
		}
		got = true
		if it.FullName == nil || *it.FullName != "Nguyễn Thị Hoa" {
			t.Errorf("item thành viên phải có full_name: %+v", it)
		}
		if it.AvatarURL == nil || *it.AvatarURL != "https://img.test/hoa.png" {
			t.Errorf("item thành viên phải có avatar_url: %+v", it)
		}
		if it.UserName == "" {
			t.Errorf("vẫn giữ user_name: %+v", it)
		}
	}
	if !got {
		t.Fatalf("không thấy thành viên trong danh sách: %+v", res.Members)
	}
}

// Vòng 2 mục 4: người xem KHÔNG phải thành viên ACTIVE thấy thành viên có hồ sơ `hidden` ẩn danh; thành viên cùng
// nhóm vẫn thấy tên thật.
func TestListMembers_HoSoAn_AnDanhVoiNguoiNgoai_ThanhVienThayTenThat(t *testing.T) {
	fx := newGroupFx(t)
	owner := guardUser(t, fx.db, "owner")
	g := fx.group(owner, model.GroupPrivacyPublic, 10)
	hidden, shown, outsider := guardUser(t, fx.db, "hidden"), guardUser(t, fx.db, "shown"), guardUser(t, fx.db, "outsider")
	fx.member(g.ID, hidden, model.GroupRoleMember, model.GroupMemberActive)
	fx.member(g.ID, shown, model.GroupRoleMember, model.GroupMemberActive)
	for _, u := range []uuid.UUID{hidden, shown} {
		if err := fx.db.Model(&model.User{}).Where("id = ?", u).
			Updates(map[string]any{"full_name": "Tên Thật " + u.String()[:4], "avatar_url": "https://img.test/" + u.String()[:4]}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := fx.db.Create(&model.UserPreference{UserID: hidden, ProfileVisibility: "hidden"}).Error; err != nil {
		t.Fatal(err)
	}
	find := func(res *dto.GroupMemberListResponse, id uuid.UUID) dto.GroupMemberResponse {
		for _, it := range res.Members {
			if it.UserID == id {
				return it
			}
		}
		t.Fatalf("không thấy %s trong danh sách", id)
		return dto.GroupMemberResponse{}
	}

	out, err := fx.svc.ListMembers(t.Context(), outsider, g.ID, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	h := find(out, hidden)
	if h.FullName == nil || *h.FullName != "Học viên" || h.UserName != "" || h.AvatarURL != nil || h.Nickname != nil {
		t.Errorf("người ngoài phải thấy thành viên ẩn hồ sơ dưới dạng ẩn danh: %+v", h)
	}
	if s := find(out, shown); s.FullName == nil || *s.FullName == "Học viên" || s.UserName == "" {
		t.Errorf("thành viên không ẩn hồ sơ vẫn hiện tên thật: %+v", s)
	}
	in, err := fx.svc.ListMembers(t.Context(), shown, g.ID, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if h := find(in, hidden); h.FullName == nil || *h.FullName == "Học viên" || h.UserName == "" {
		t.Errorf("thành viên cùng nhóm vẫn phải thấy tên thật: %+v", h)
	}
}