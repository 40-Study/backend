package service

// Lane P (câu hỏi mở 5 và 10 của rà soát phân quyền): tự điểm danh chỉ mở đúng ngày của buổi, buổi đã
// huỷ/kết thúc thì đóng; sinh buổi hàng loạt có trần khoảng ngày. Postgres thật, schema tạm riêng.
// Bỏ requireCheckInOpen khỏi StudentCheckIn (hoặc trần khỏi GenerateSessions) thì các test dưới ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestStudentCheckIn_OnlyOpenOnSessionDay(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	today := time.Now().In(sessionDayZone)

	setDay := func(id interface{}, day time.Time) {
		t.Helper()
		if err := e.f.db.Exec("UPDATE class_sessions SET date = ? WHERE id = ?", day.Format("2006-01-02"), id).Error; err != nil {
			t.Fatal(err)
		}
	}
	setStatus := func(id interface{}, status model.ClassSessionStatus) {
		t.Helper()
		if err := e.f.db.Exec("UPDATE class_sessions SET status = ? WHERE id = ?", status, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	attendanceRows := func() int64 {
		var n int64
		e.f.db.Model(&model.SessionAttendance{}).Where("student_id = ?", e.student.ID).Count(&n)
		return n
	}

	ses := s5MakeSession(t, e, e.class.ID, 1)

	// Ngày khác (mai, hôm qua, năm ngoái): 409-loại lỗi, không ghi dòng điểm danh nào.
	for name, day := range map[string]time.Time{
		"ngày mai":    today.AddDate(0, 0, 1),
		"hôm qua":     today.AddDate(0, 0, -1),
		"năm ngoái":   today.AddDate(-1, 0, 0),
		"tuần sau đó": today.AddDate(0, 0, 7),
	} {
		setDay(ses.ID, day)
		if _, err := svc.StudentCheckIn(ctx, ses.ID, e.student.ID); !errors.Is(err, ErrCheckInOutsideSessionDay) {
			t.Errorf("%s: err=%v, muốn ErrCheckInOutsideSessionDay", name, err)
		}
	}
	if n := attendanceRows(); n != 0 {
		t.Fatalf("check-in bị từ chối vẫn ghi %d dòng điểm danh", n)
	}

	// Đúng ngày: qua. Điểm danh lại trong cùng buổi vẫn cập nhật được (idempotent như trước).
	setDay(ses.ID, today)
	if _, err := svc.StudentCheckIn(ctx, ses.ID, e.student.ID); err != nil {
		t.Fatalf("check-in đúng ngày bị chặn nhầm: %v", err)
	}
	if _, err := svc.StudentCheckIn(ctx, ses.ID, e.student.ID); err != nil {
		t.Fatalf("check-in lần hai cùng ngày bị chặn nhầm: %v", err)
	}
	if n := attendanceRows(); n != 1 {
		t.Fatalf("điểm danh = %d dòng, muốn 1", n)
	}

	// Buổi đã huỷ hoặc đã kết thúc: đóng dù đúng ngày.
	other := s5MakeSession(t, e, e.class.ID, 2)
	setDay(other.ID, today)
	for _, status := range []model.ClassSessionStatus{model.SessionCancelled, model.SessionCompleted} {
		setStatus(other.ID, status)
		if _, err := svc.StudentCheckIn(ctx, other.ID, e.student.ID); !errors.Is(err, ErrSessionClosedForCheckIn) {
			t.Errorf("buổi %s: err=%v, muốn ErrSessionClosedForCheckIn", status, err)
		}
	}
	setStatus(other.ID, model.SessionInProgress)
	if _, err := svc.StudentCheckIn(ctx, other.ID, e.student.ID); err != nil {
		t.Errorf("buổi đang diễn ra bị chặn nhầm: %v", err)
	}
}

// Ranh giới ngày theo giờ Việt Nam: 23:59 ICT còn trong ngày buổi, 00:00 ICT hôm sau đã sang ngày khác.
func TestRequireCheckInOpen_DayBoundaryIsVietnamTime(t *testing.T) {
	session := &model.ClassSession{Date: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Status: model.SessionScheduled}
	cases := map[string]struct {
		now  time.Time
		want error
	}{
		"00:00 ICT đầu ngày":      {time.Date(2026, 10, 1, 0, 0, 0, 0, sessionDayZone), nil},
		"23:59 ICT cuối ngày":     {time.Date(2026, 10, 1, 23, 59, 59, 0, sessionDayZone), nil},
		"00:00 ICT hôm sau":       {time.Date(2026, 10, 2, 0, 0, 0, 0, sessionDayZone), ErrCheckInOutsideSessionDay},
		"23:59 ICT hôm trước":     {time.Date(2026, 9, 30, 23, 59, 59, 0, sessionDayZone), ErrCheckInOutsideSessionDay},
		"17:30 UTC = 00:30 ICT+1": {time.Date(2026, 10, 1, 17, 30, 0, 0, time.UTC), ErrCheckInOutsideSessionDay},
		"16:30 UTC = 23:30 ICT":   {time.Date(2026, 10, 1, 16, 30, 0, 0, time.UTC), nil},
	}
	for name, c := range cases {
		if err := requireCheckInOpen(session, c.now); !errors.Is(err, c.want) {
			t.Errorf("%s: err=%v, muốn %v", name, err, c.want)
		}
	}
}

func TestGenerateSessions_RejectsRangeOverOneYear(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	s5MakeSchedule(t, e, e.class.ID)

	var before int64
	e.f.db.Model(&model.ClassSession{}).Where("class_id = ?", e.class.ID).Count(&before)

	_, err := svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, dto.GenerateSessionsDTO{StartDate: "2026-01-01", EndDate: "2030-01-01"})
	if !errors.Is(err, ErrGenerateRangeTooLong) {
		t.Fatalf("khoảng 4 năm: err=%v, muốn ErrGenerateRangeTooLong", err)
	}
	var after int64
	e.f.db.Model(&model.ClassSession{}).Where("class_id = ?", e.class.ID).Count(&after)
	if after != before {
		t.Fatalf("khoảng quá dài vẫn sinh %d buổi", after-before)
	}

	// Khoảng vài tuần vẫn sinh buổi bình thường (trần chỉ chặn khoảng quá một năm).
	if _, err := svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, dto.GenerateSessionsDTO{StartDate: "2026-10-01", EndDate: "2026-10-28"}); err != nil {
		t.Fatalf("khoảng 4 tuần bị chặn nhầm: %v", err)
	}
}
