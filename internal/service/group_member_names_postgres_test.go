package service

// Lane web cần hiển thị tên thật của thành viên nhóm: item danh sách thành viên có full_name và avatar_url
// (trước đây chỉ có user_name).

import (
	"testing"

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