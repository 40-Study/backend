package service

// QA5 round 2 (Postgres thật, schema tạm). R2: GET /me/attendances thêm class_name + session_title. R3: mục thời
// khoá biểu thêm teacher_name (giảng viên chính của lớp). R4: CancelSession từ chối buổi đã tới giờ bắt đầu
// (ICT) với ErrSessionAlreadyStarted. Bỏ phần tương ứng trong schedule_service / timetable_organization thì ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func TestQA5R2_MyAttendances_CarryClassAndSessionTitle_Postgres(t *testing.T) {
	e := newS4ClassEnv(t)
	svc := newS5ScheduleSvc(e)
	topic := "Biến và kiểu dữ liệu"
	withTopic := s5MakeSession(t, e, e.class.ID, 1)
	withTopic.Topic = &topic
	if err := e.f.db.Save(&withTopic).Error; err != nil {
		t.Fatal(err)
	}
	noTopic := s5MakeSession(t, e, e.class.ID, 2)
	for _, sess := range []model.ClassSession{withTopic, noTopic} {
		att := model.SessionAttendance{SessionID: sess.ID, StudentID: e.student.ID, Status: model.AttendancePresent}
		if err := e.f.db.Create(&att).Error; err != nil {
			t.Fatal(err)
		}
	}

	rows, total, err := svc.GetMyAttendances(context.Background(), e.student.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("muốn 2 dòng điểm danh, có total=%d len=%d", total, len(rows))
	}
	titles := map[uuid.UUID]string{}
	for _, r := range rows {
		if r.ClassName != e.class.Name {
			t.Errorf("class_name muốn %q, có %q", e.class.Name, r.ClassName)
		}
		titles[r.SessionID] = r.SessionTitle
	}
	if titles[withTopic.ID] != topic {
		t.Errorf("buổi có chủ đề: session_title muốn %q, có %q", topic, titles[withTopic.ID])
	}
	if titles[noTopic.ID] != "Buổi 2" {
		t.Errorf("buổi không chủ đề: session_title muốn \"Buổi 2\", có %q", titles[noTopic.ID])
	}
}

func TestQA5R3_TimetableEntries_CarryMainTeacherName_Postgres(t *testing.T) {
	e := newTimetableOrgEnv(t)
	ctx := context.Background()
	// Trợ giảng được gán TRƯỚC vẫn không được thay giảng viên chính.
	assistant := e.f.user("assistant")
	if err := e.f.db.Create(&model.TeacherClass{TeacherID: assistant.ID, ClassID: e.class.ID, Role: "assistant",
		AssignedAt: time.Now().Add(-48 * time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.GetMyTimetable(ctx, e.coTeacher.ID, "TEACHER", dto.TimetableOrgFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) == 0 {
		t.Fatal("muốn có mục thời khoá biểu")
	}
	for _, en := range got.Entries {
		if en.TeacherName != e.coTeacher.UserName {
			t.Errorf("lớp %s: teacher_name muốn %q (giảng viên chính), có %q", en.ClassID, e.coTeacher.UserName, en.TeacherName)
		}
	}

	// Lớp chưa có giảng viên chính: vắng, không bịa tên.
	orphan := model.Class{Name: "QA5-no-primary", CourseID: &e.course.ID, Status: "active"}
	if err := e.f.db.Create(&orphan).Error; err != nil {
		t.Fatal(err)
	}
	names, err := e.svc.repo.GetClassMainTeacherNames(ctx, []uuid.UUID{orphan.ID, e.class.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, has := names[orphan.ID]; has || names[e.class.ID] != e.coTeacher.UserName {
		t.Errorf("map tên giảng viên chính sai: %v", names)
	}
}

func TestQA5R4_CancelSession_RefusesStartedSession_Postgres(t *testing.T) {
	e := newS4ClassEnv(t)
	svc := newS5ScheduleSvc(e)
	ctx := context.Background()
	today := time.Now().In(sessionDayZone)
	mk := func(n int, day time.Time, start, end string) model.ClassSession {
		s := model.ClassSession{ClassID: e.class.ID, SessionNumber: n, Date: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
			StartTime: model.TimeOfDay(start), EndTime: model.TimeOfDay(end), Status: model.SessionScheduled}
		if err := e.f.db.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	yesterday := mk(1, today.AddDate(0, 0, -1), "08:00", "09:00")
	earlierToday := mk(2, today, "00:00", "00:30") // đã qua giờ bắt đầu ở mọi thời điểm trong ngày trừ đúng 00:00:00
	tomorrow := mk(3, today.AddDate(0, 0, 1), "08:00", "09:00")

	for name, s := range map[string]model.ClassSession{"hôm qua": yesterday, "sáng nay đã qua giờ": earlierToday} {
		err := svc.CancelSession(ctx, s.ID, e.coTeacher.ID, false, "x")
		if !errors.Is(err, ErrSessionAlreadyStarted) {
			t.Errorf("%s: muốn ErrSessionAlreadyStarted, có %v", name, err)
		}
		var after model.ClassSession
		if err := e.f.db.First(&after, "id = ?", s.ID).Error; err != nil {
			t.Fatal(err)
		}
		if after.Status != model.SessionScheduled || after.CancelledAt != nil {
			t.Errorf("%s: buổi bị từ chối không được đổi trạng thái: %s", name, after.Status)
		}
	}
	if err := svc.CancelSession(ctx, tomorrow.ID, e.coTeacher.ID, false, "nghỉ"); err != nil {
		t.Fatalf("buổi ngày mai phải huỷ được: %v", err)
	}
	var after model.ClassSession
	if err := e.f.db.First(&after, "id = ?", tomorrow.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != model.SessionCancelled {
		t.Errorf("buổi ngày mai phải ở trạng thái cancelled, có %s", after.Status)
	}
}
