package service

// QA4-N10: phụ huynh có 2 con cùng lớp nhận nguyên văn nhắc nhở của học viên ("… Hãy vào lớp đúng giờ.") và
// không biết buổi của con nào. Giờ: một nhắc nhở cho MỖI con, ghi tên con, trỏ về màn chi tiết của con đó.
// Con có tài khoản bị khoá thì không được nhắc. Postgres THẬT. Mutation đã kiểm: đổi lại LIMIT 1 / bỏ tên con khỏi
// nội dung / bỏ lọc u.is_active làm test ĐỎ.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func TestLessonReminder_ParentGetsOneNamedNoticePerChild_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	teacher, parent := f.user("n10-teacher"), f.user("n10-parent")
	kidA, kidB, gone := f.user("n10-kid-a"), f.user("n10-kid-b"), f.user("n10-kid-gone")
	fullName := "Lê Văn C"
	must(f.db.Model(&model.User{}).Where("id = ?", kidA.ID).Update("full_name", fullName).Error)
	class := model.Class{Name: "Tiếng Anh 6", Status: "active"}
	must(f.db.Create(&class).Error)
	must(f.db.Create(&model.TeacherClass{TeacherID: teacher.ID, ClassID: class.ID}).Error)
	for _, kid := range []model.User{kidA, kidB, gone} {
		must(f.db.Create(&model.StudentClass{StudentID: kid.ID, ClassID: class.ID, Status: "active"}).Error)
		must(f.db.Create(&model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: kid.ID, Status: "active", CanViewAttendance: true}).Error)
	}
	// Review #148: con đã khoá tài khoản (vẫn active trong lớp) không sinh nhắc nhở cho phụ huynh.
	must(f.db.Model(&model.User{}).Where("id = ?", gone.ID).Update("is_active", false).Error)
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	start, end := now.Add(15*time.Minute), now.Add(75*time.Minute)
	must(f.db.Create(&model.LivestreamSession{HostID: teacher.ID, ClassID: class.ID, Title: "Unit 3", RoomName: uuid.NewString(),
		Status: model.LivestreamStatusScheduled, ScheduledAt: &start, ScheduledEndAt: &end, MaxViewers: 30}).Error)

	svc := NewLessonReminderService(f.db, nil)
	must(svc.Sweep(context.Background(), now))
	must(svc.Sweep(context.Background(), now)) // lặp lại: id tất định, không trùng

	var notices []model.Notification
	must(f.db.Where("user_id = ? AND notification_type = ?", parent.ID, "lesson_reminder").Find(&notices).Error)
	want := map[uuid.UUID]string{
		kidA.ID: fullName + " có buổi Unit 3 lúc 10:15 · 15/10.",
		kidB.ID: kidB.UserName + " có buổi Unit 3 lúc 10:15 · 15/10.",
	}
	if len(notices) != len(want) {
		t.Fatalf("phụ huynh nhận %d nhắc nhở, muốn %d (một cho mỗi con): %+v", len(notices), len(want), notices)
	}
	for _, n := range notices {
		if n.ReferenceType == nil || *n.ReferenceType != "child_schedule" || n.ReferenceID == nil {
			t.Fatalf("đích sai: %+v", n)
		}
		if n.Content != want[*n.ReferenceID] {
			t.Errorf("con %s: nội dung %q, muốn %q", *n.ReferenceID, n.Content, want[*n.ReferenceID])
		}
		delete(want, *n.ReferenceID)
	}
	if len(want) != 0 {
		t.Fatalf("thiếu nhắc nhở cho con: %v", want)
	}
}
