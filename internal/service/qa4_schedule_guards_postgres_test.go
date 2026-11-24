package service

// QA4-T06 / QA4-T07 (Postgres thật, schema tạm): sinh buổi trả danh sách ngày bị bỏ qua kèm lý do; điểm danh buổi có ngày
// sau hôm nay (giờ Việt Nam) bị từ chối. Bỏ skipped khỏi GenerateSessions / requireSessionStarted thì test ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestQA4_GenerateSessions_ReportsSkippedDates(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	first := day("2031-03-03")
	sch := model.ClassSchedule{ClassID: e.class.ID, DayOfWeek: int(first.Weekday()), StartTime: "18:00", EndTime: "20:00",
		IsActive: true, EffectiveFrom: first}
	if err := e.f.db.Create(&sch).Error; err != nil {
		t.Fatal(err)
	}
	// Lớp khác của cùng giảng viên (co-teacher) học trùng giờ vào tuần thứ hai.
	other := model.Class{Name: "QA4-other", Status: "active"}
	if err := e.f.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.f.db.Create(&model.TeacherClass{TeacherID: e.coTeacher.ID, ClassID: other.ID, Role: "primary"}).Error; err != nil {
		t.Fatal(err)
	}
	clash := model.ClassSession{ClassID: other.ID, SessionNumber: 1, Date: first.AddDate(0, 0, 7), StartTime: "18:00", EndTime: "19:00", Status: model.SessionScheduled}
	if err := e.f.db.Create(&clash).Error; err != nil {
		t.Fatal(err)
	}

	out, skipped, err := svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-03-17"})
	if err != nil {
		t.Fatalf("sinh buổi: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("sinh %d buổi, muốn 2 (bỏ tuần trùng)", len(out))
	}
	want := []dto.SkippedSessionDTO{{Date: "2031-03-10", Reason: dto.SkippedSessionOverlap}}
	assertSkipped(t, "lần 1", skipped, want)

	// Chạy lại trên khoảng rộng hơn: buổi đã sinh là duplicate, buổi trùng lớp khác vẫn là overlap, tuần mới được sinh.
	out, skipped, err = svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-03-24"})
	if err != nil {
		t.Fatalf("sinh lại: %v", err)
	}
	if len(out) != 1 || out[0].Date != "2031-03-24" {
		t.Fatalf("sinh lại được %v, muốn đúng buổi 2031-03-24", out)
	}
	assertSkipped(t, "lần 2", skipped, []dto.SkippedSessionDTO{
		{Date: "2031-03-03", Reason: dto.SkippedSessionDuplicate},
		{Date: "2031-03-10", Reason: dto.SkippedSessionOverlap},
		{Date: "2031-03-17", Reason: dto.SkippedSessionDuplicate},
	})

	// Mọi ngày đều bị bỏ qua: vẫn 409 nhưng danh sách lý do đi kèm. Buổi 2031-03-24 bị huỷ thì báo "cancelled".
	if err := e.f.db.Model(&model.ClassSession{}).Where("class_id = ? AND date = ?", e.class.ID, "2031-03-24").
		Update("status", model.SessionCancelled).Error; err != nil {
		t.Fatal(err)
	}
	out, skipped, err = svc.GenerateSessions(ctx, e.class.ID, e.owner.ID, false, dto.GenerateSessionsDTO{StartDate: "2031-03-03", EndDate: "2031-03-24"})
	if !errors.Is(err, ErrSessionOverlap) || out != nil {
		t.Fatalf("sinh lại khoảng đã sinh hết: out=%v err=%v, muốn ErrSessionOverlap", out, err)
	}
	assertSkipped(t, "lần 3 (409)", skipped, []dto.SkippedSessionDTO{
		{Date: "2031-03-03", Reason: dto.SkippedSessionDuplicate},
		{Date: "2031-03-10", Reason: dto.SkippedSessionOverlap},
		{Date: "2031-03-17", Reason: dto.SkippedSessionDuplicate},
		{Date: "2031-03-24", Reason: dto.SkippedSessionCancelled},
	})
}

func assertSkipped(t *testing.T, label string, got, want []dto.SkippedSessionDTO) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: skipped=%v, muốn %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: skipped[%d]=%v, muốn %v", label, i, got[i], want[i])
		}
	}
}

func TestQA4_Attendance_RefusedBeforeSessionDay(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	today := time.Now().In(sessionDayZone)
	mk := func(n int, day time.Time) model.ClassSession {
		s := model.ClassSession{ClassID: e.class.ID, SessionNumber: n, Date: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
			StartTime: "08:00", EndTime: "09:00", Status: model.SessionScheduled}
		if err := e.f.db.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	future, now := mk(1, today.AddDate(0, 0, 7)), mk(2, today)
	mark := dto.MarkAttendanceDTO{StudentID: e.student.ID.String(), Status: "present"}

	if _, err := svc.MarkAttendance(ctx, future.ID, mark, e.owner.ID, false); !errors.Is(err, ErrSessionNotStarted) {
		t.Errorf("điểm danh buổi tuần sau: err=%v, muốn ErrSessionNotStarted", err)
	}
	if _, err := svc.BulkMarkAttendance(ctx, future.ID, dto.BulkMarkAttendanceDTO{Attendances: []dto.MarkAttendanceDTO{mark}}, e.owner.ID, false); !errors.Is(err, ErrSessionNotStarted) {
		t.Errorf("điểm danh hàng loạt buổi tuần sau: err=%v, muốn ErrSessionNotStarted", err)
	}
	legacy := model.SessionAttendance{SessionID: future.ID, StudentID: e.student.ID, Status: model.AttendancePresent}
	if err := e.f.db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	absent := "absent"
	if _, err := svc.UpdateAttendance(ctx, future.ID, legacy.ID, dto.UpdateSessionAttendanceDTO{Status: &absent}, e.owner.ID, false); !errors.Is(err, ErrSessionNotStarted) {
		t.Errorf("sửa điểm danh buổi tuần sau: err=%v, muốn ErrSessionNotStarted", err)
	}
	var n int64
	e.f.db.Model(&model.SessionAttendance{}).Where("session_id = ?", future.ID).Count(&n)
	if n != 1 {
		t.Errorf("buổi tương lai có %d dòng điểm danh, muốn chỉ dòng cũ", n)
	}
	if _, err := svc.MarkAttendance(ctx, now.ID, mark, e.owner.ID, false); err != nil {
		t.Errorf("điểm danh buổi hôm nay: %v", err)
	}
}

// Ngày của buổi so theo giờ Việt Nam: 23:30 ICT ngày 24 thì buổi ngày 25 chưa tới; 00:30 ICT ngày 25 thì đã tới.
func TestQA4_RequireSessionStarted_UsesVietnamDay(t *testing.T) {
	session := &model.ClassSession{Date: time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)}
	if err := requireSessionStarted(session, time.Date(2026, 11, 24, 16, 30, 0, 0, time.UTC)); !errors.Is(err, ErrSessionNotStarted) {
		t.Errorf("23:30 ICT hôm trước: err=%v, muốn ErrSessionNotStarted", err)
	}
	if err := requireSessionStarted(session, time.Date(2026, 11, 24, 17, 30, 0, 0, time.UTC)); err != nil {
		t.Errorf("00:30 ICT đúng ngày: %v", err)
	}
	if err := requireSessionStarted(session, time.Date(2026, 12, 3, 3, 0, 0, 0, time.UTC)); err != nil {
		t.Errorf("buổi đã qua: %v", err)
	}
}
