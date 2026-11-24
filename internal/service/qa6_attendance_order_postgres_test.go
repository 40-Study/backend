package service

// QA6-PG-07: lịch sử điểm danh xếp theo NGÀY BUỔI HỌC mới nhất trước (không theo lúc ghi điểm danh), có tie-breaker ổn định.
// QA6-L5: buổi đã xoá mềm vẫn trả "Buổi N" và tên lớp (trước đây "Buổi 0" rỗng).
// Mutation đã kiểm: về lại Order("created_at DESC") làm ca thứ tự ĐỎ; bỏ Unscoped ở Preload làm ca xoá mềm ĐỎ.

import (
	"context"
	"testing"
	"time"

	"study.com/v1/internal/model"
)

func TestQA6_MyAttendances_TheoNgayBuoiHocVaBuoiXoaMem_Postgres(t *testing.T) {
	e := newS4ClassEnv(t)
	svc := newS5ScheduleSvc(e)
	ctx := context.Background()
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	mk := func(n, dayOffset int) model.ClassSession {
		s := model.ClassSession{ClassID: e.class.ID, SessionNumber: n, Date: base.AddDate(0, 0, dayOffset), StartTime: "08:00", EndTime: "09:00", Status: model.SessionCompleted}
		if err := e.f.db.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	// Ghi điểm danh theo thứ tự KHÁC thứ tự ngày: buổi giữa trước, buổi cũ nhất, rồi buổi mới nhất.
	mid, oldest, newest, deleted := mk(2, 5), mk(1, 0), mk(3, 10), mk(4, 7)
	for _, s := range []model.ClassSession{mid, oldest, newest, deleted} {
		att := model.SessionAttendance{SessionID: s.ID, StudentID: e.student.ID, Status: model.AttendancePresent}
		if err := e.f.db.Create(&att).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := e.f.db.Delete(&deleted).Error; err != nil { // xoá mềm buổi số 4
		t.Fatal(err)
	}

	rows, total, err := svc.GetMyAttendances(ctx, e.student.ID, 1, 20)
	if err != nil || total != 4 || len(rows) != 4 {
		t.Fatalf("muốn 4 dòng, có total=%d len=%d err=%v", total, len(rows), err)
	}
	wantOrder := []string{newest.ID.String(), deleted.ID.String(), mid.ID.String(), oldest.ID.String()}
	for i, r := range rows {
		if r.SessionID.String() != wantOrder[i] {
			t.Errorf("vị trí %d: session %s, muốn %s (ngày buổi học giảm dần)", i, r.SessionID, wantOrder[i])
		}
	}
	if got := rows[1]; got.SessionTitle != "Buổi 4" || got.ClassName != e.class.Name {
		t.Errorf("buổi xoá mềm: title=%q class=%q, muốn \"Buổi 4\" và %q", got.SessionTitle, got.ClassName, e.class.Name)
	}

	// Phân trang không xáo trộn: trang 2 cỡ 2 là hai buổi cũ hơn, không trùng trang 1.
	p1, _, _ := svc.GetMyAttendances(ctx, e.student.ID, 1, 2)
	p2, _, _ := svc.GetMyAttendances(ctx, e.student.ID, 2, 2)
	if len(p1) != 2 || len(p2) != 2 || p1[0].SessionID != newest.ID || p2[1].SessionID != oldest.ID {
		t.Errorf("phân trang sai: p1=%v p2=%v", p1, p2)
	}
}
