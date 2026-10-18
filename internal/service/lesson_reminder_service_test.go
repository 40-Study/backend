package service

import (
	"context"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"sync"
	"testing"
	"time"
)

func TestLessonReminder_AudienceRetryRescheduleCancel_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	host, teacher, student, other, parent, revoked := f.user("host"), f.user("teacher"), f.user("student"), f.user("other"), f.user("parent"), f.user("revoked")
	class := model.Class{Name: "Reminder QA", Status: "active"}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.db.Create(&class).Error)
	for _, r := range []model.TeacherClass{{TeacherID: host.ID, ClassID: class.ID}, {TeacherID: teacher.ID, ClassID: class.ID}} {
		must(f.db.Create(&r).Error)
	}
	for _, r := range []model.StudentClass{{StudentID: student.ID, ClassID: class.ID, Status: "active"}, {StudentID: other.ID, ClassID: class.ID, Status: "dropped"}} {
		must(f.db.Create(&r).Error)
	}
	for _, r := range []model.ParentStudentRelation{{ParentUserID: parent.ID, StudentUserID: student.ID, Status: "active", CanViewAttendance: true}, {ParentUserID: revoked.ID, StudentUserID: student.ID, Status: "revoked", CanViewAttendance: true}} {
		must(f.db.Create(&r).Error)
	}
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	start, end := now.Add(15*time.Minute), now.Add(75*time.Minute)
	live := model.LivestreamSession{HostID: host.ID, ClassID: class.ID, Title: "Python", RoomName: uuid.NewString(), Status: model.LivestreamStatusScheduled, ScheduledAt: &start, ScheduledEndAt: &end, MaxViewers: 30}
	must(f.db.Create(&live).Error)
	svc := NewLessonReminderService(f.db, nil)
	sweep := func(at time.Time) { t.Helper(); must(svc.Sweep(context.Background(), at)) }
	count := func(want int64) {
		t.Helper()
		var n int64
		must(f.db.Model(&model.Notification{}).Where("notification_type = ?", "lesson_reminder").Count(&n).Error)
		if n != want {
			t.Fatalf("notices=%d want=%d", n, want)
		}
	}
	sweep(now)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.Sweep(context.Background(), now); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	count(4)
	var notices []model.Notification
	must(f.db.Find(&notices).Error)
	for _, n := range notices {
		if n.UserID == other.ID || n.UserID == revoked.ID {
			t.Fatal("excluded user notified")
		}
		if n.UserID == parent.ID && (n.ReferenceType == nil || *n.ReferenceType != "child_schedule" || n.ReferenceID == nil || *n.ReferenceID != student.ID) {
			t.Fatalf("parent target incorrect: %+v", n)
		}
		if n.Content != "Python · 10:15 · 15/10. Chuẩn bị vào lớp nhé." {
			t.Fatalf("VN time incorrect: %s", n.Content)
		}
	}

	parentSvc := &ParentDashboardService{parentStudentRepo: repository.NewParentStudentRepository(f.db), scheduleRepo: repository.NewScheduleRepository(f.db)}
	calendar, err := parentSvc.GetChildTimetable(context.Background(), parent.ID, student.ID)
	must(err)
	if len(calendar.Entries) != 1 || calendar.Entries[0].SessionID == nil || *calendar.Entries[0].SessionID != live.ID {
		t.Fatalf("parent missing class livestream: %+v", calendar)
	}
	if calendar.Entries[0].StartTime != "10:15" {
		t.Fatalf("parent calendar VN time: %s", calendar.Entries[0].StartTime)
	}
	if _, err := parentSvc.GetChildTimetable(context.Background(), revoked.ID, student.ID); err == nil {
		t.Fatal("revoked parent accessed calendar")
	}
	must(f.db.Model(&model.ParentStudentRelation{}).Where("parent_user_id = ? AND student_user_id = ?", parent.ID, student.ID).Update("can_view_attendance", false).Error)
	if _, err := parentSvc.GetChildTimetable(context.Background(), parent.ID, student.ID); err == nil {
		t.Fatal("calendar permission ignored")
	}
	must(f.db.Model(&model.ParentStudentRelation{}).Where("parent_user_id = ? AND student_user_id = ?", parent.ID, student.ID).Update("can_view_attendance", true).Error)
	oldStart := start
	moved, movedEnd := start.Add(time.Hour), end.Add(time.Hour)
	must(f.db.Model(&live).Updates(map[string]interface{}{"scheduled_at": moved, "scheduled_end_at": movedEnd}).Error)
	sweep(oldStart)
	count(4)
	sweep(moved)
	sweep(moved.Add(30 * time.Second))
	count(8)
	must(f.db.Delete(&live).Error)
	sweep(moved.Add(time.Minute))
	count(8)
}

func TestLessonReminder_WeeklyCancelledOccurrence_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	teacher, student := f.user("teacher"), f.user("student")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	class := model.Class{Name: "Weekly", Status: "active"}
	must(f.db.Create(&class).Error)
	must(f.db.Create(&model.TeacherClass{TeacherID: teacher.ID, ClassID: class.ID}).Error)
	must(f.db.Create(&model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active"}).Error)
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	must(f.db.Exec(`INSERT INTO class_schedules (id,class_id,day_of_week,start_time,end_time,is_active,effective_from) VALUES (?, ?, 4, '10:00','11:00',true,'2026-10-01')`, uuid.New(), class.ID).Error)
	svc := NewLessonReminderService(f.db, nil)
	must(svc.Sweep(context.Background(), now))
	var n int64
	must(f.db.Model(&model.Notification{}).Count(&n).Error)
	if n != 2 {
		t.Fatalf("weekly notices=%d", n)
	}
	must(f.db.Exec(`INSERT INTO class_sessions (id,class_id,schedule_id,session_number,date,start_time,end_time,status) SELECT ?,class_id,id,2,'2026-10-22','10:00','11:00','cancelled' FROM class_schedules WHERE class_id = ?`, uuid.New(), class.ID).Error)
	must(svc.Sweep(context.Background(), now.AddDate(0, 0, 7)))
	must(f.db.Model(&model.Notification{}).Count(&n).Error)
	if n != 2 {
		t.Fatalf("cancelled occurrence notified: %d", n)
	}
}
