package service

// W2-B: bảng xếp hạng THEO LỚP (class_id) dùng cùng luật riêng tư `leaderboard_display` với bảng toàn hệ thống.
// Field của backend là user_preferences.leaderboard_display (name | username | anonymous); `leaderboard_visibility`
// là tên web từng dùng nhầm, không phải field nào của backend và không có field riêng cho lớp/khoá. Test khoá việc
// bảng của lớp không bỏ qua cài đặt này: bạn cùng lớp không thấy tên thật của người đặt ẩn danh.

import (
	"context"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ListMyClassBoards phải trùng định nghĩa CanViewClassBoard: học viên đang ghi danh và giảng viên có lớp trong ô chọn;
// học viên đã rời lớp, người lạ và lớp đã xoá mềm thì không.
func TestW2B_ListMyClassBoards_MatchesWhoCanView(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	db := e.f.db
	svc := NewLeaderboardService(repository.NewLeaderboardRepository(db))
	names := func(userID model.User) []string {
		out, err := svc.ListMyClassBoards(ctx, userID.ID)
		if err != nil {
			t.Fatal(err)
		}
		if out == nil {
			t.Fatal("phải trả mảng rỗng, không phải nil")
		}
		var n []string
		for _, c := range out {
			n = append(n, c.Name)
		}
		return n
	}

	if got := names(e.student); len(got) != 1 || got[0] != e.class.Name {
		t.Errorf("học viên của lớp thấy %v, muốn [%s]", got, e.class.Name)
	}
	if got := names(e.coTeacher); len(got) != 1 || got[0] != e.class.Name {
		t.Errorf("giảng viên của lớp thấy %v, muốn [%s]", got, e.class.Name)
	}
	stranger := e.f.user("lbc-stranger")
	if got := names(stranger); len(got) != 0 {
		t.Errorf("người lạ thấy %v, muốn rỗng", got)
	}
	// Rời lớp (status != active) thì mất lựa chọn, đồng thời CanViewClassBoard cũng từ chối: hai luật không lệch nhau.
	if err := db.Model(&model.StudentClass{}).Where("student_id = ? AND class_id = ?", e.student.ID, e.class.ID).Update("status", "dropped").Error; err != nil {
		t.Fatal(err)
	}
	if got := names(e.student); len(got) != 0 {
		t.Errorf("học viên đã rời lớp vẫn thấy %v", got)
	}
	if err := db.Delete(&model.Class{}, "id = ?", e.class.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := names(e.coTeacher); len(got) != 0 {
		t.Errorf("lớp đã xoá mềm vẫn nằm trong ô chọn của giảng viên: %v", got)
	}
}

func TestW2B_ClassLeaderboard_RespectsLeaderboardDisplay(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	db := e.f.db

	anon, plain := e.f.user("lbd-anon"), e.f.user("lbd-plain")
	fullAnon := "Trần Thị Bí Mật"
	for _, u := range []model.User{anon, plain} {
		db.Model(&model.User{}).Where("id = ?", u.ID).Update("is_active", true)
		grantSystemRole(t, db, u.ID, "STUDENT")
		if err := db.Create(&model.StudentClass{StudentID: u.ID, ClassID: e.class.ID, Status: "active"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Model(&model.User{}).Where("id = ?", anon.ID).Update("full_name", fullAnon)
	grantSystemRole(t, db, e.student.ID, "STUDENT")
	for i, u := range []model.User{anon, plain, e.student} {
		if err := db.Create(&model.UserPoint{UserID: u.ID, TotalPoints: 500 - i*10}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.UserPreference{UserID: anon.ID, LeaderboardDisplay: LeaderboardDisplayAnonymous}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewLeaderboardService(repository.NewLeaderboardRepository(db))
	classID := e.class.ID
	rowOf := func(viewerID *model.User) (found bool, display string, hasID bool) {
		var viewer *LeaderboardViewer
		if viewerID != nil {
			viewer = &LeaderboardViewer{UserID: viewerID.ID}
		}
		resp, err := svc.GetLeaderboard(ctx, "all_time", 100, viewer, &classID)
		if err != nil {
			t.Fatalf("xem bảng theo lớp: %v", err)
		}
		for _, en := range resp.Entries {
			if en.Points == 500 { // điểm riêng của anon
				return true, en.DisplayName, en.UserID != nil
			}
		}
		return false, "", false
	}

	// Bạn cùng lớp: thấy nhãn ẩn danh, không thấy tên thật và không có id dẫn tới hồ sơ.
	found, display, hasID := rowOf(&plain)
	if !found {
		t.Fatal("người đặt ẩn danh mất khỏi bảng của lớp")
	}
	if display != AnonymousLearnerLabel || hasID {
		t.Errorf("bạn cùng lớp thấy display=%q hasID=%v, muốn %q và không có id", display, hasID, AnonymousLearnerLabel)
	}
	// Chính chủ vẫn thấy tên thật của mình.
	if _, display, hasID = rowOf(&anon); display != fullAnon || !hasID {
		t.Errorf("chính chủ thấy display=%q hasID=%v, muốn %q và có id", display, hasID, fullAnon)
	}
}
