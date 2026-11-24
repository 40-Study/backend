package service

// QA5 backend lane (Postgres THẬT, schema tạm): C1 my_submission, C3 thống kê điểm danh toàn bộ, Q5-PG-02 lịch sắp
// tới, Q5-PG-13 tách chờ chấm / cần làm lại, C5 cảnh báo trùng lịch giảng viên, Q5-T07 nhắc nhở giọng giảng viên,
// C4 tra tên đối tượng nhật ký.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func qa5Must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// ---- C3 / Q5-PG-01: thống kê trên toàn bộ bản ghi ----

func TestQA5_ParentAttendanceStats_CoverAllRecordsNotThePage(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	parent, student := f.user("pg01-parent"), f.user("pg01-student")
	class := model.Class{Name: "QA5 PG01", Status: "active"}
	qa5Must(t, f.db.Create(&class).Error)
	qa5Must(t, f.db.Create(&model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: student.ID, Status: "active", CanViewAttendance: true}).Error)
	statuses := []model.AttendanceStatus{
		model.AttendancePresent, model.AttendancePresent, model.AttendancePresent,
		model.AttendanceAbsent, model.AttendanceAbsent, model.AttendanceLate, model.AttendanceExcused,
	}
	for i, st := range statuses {
		sess := model.ClassSession{ClassID: class.ID, SessionNumber: i + 1, Date: time.Now().AddDate(0, 0, -(i + 1)), StartTime: "18:00", EndTime: "19:00", Status: model.SessionCompleted}
		qa5Must(t, f.db.Create(&sess).Error)
		qa5Must(t, f.db.Create(&model.SessionAttendance{SessionID: sess.ID, StudentID: student.ID, Status: st}).Error)
	}
	svc := &ParentDashboardService{parentStudentRepo: repository.NewParentStudentRepository(f.db), scheduleRepo: repository.NewScheduleRepository(f.db)}

	got, err := svc.GetChildAttendance(ctx, parent.ID, student.ID, 1, 2)
	qa5Must(t, err)
	if len(got.Records) != 2 || got.Total != 7 {
		t.Fatalf("trang 1: %d bản ghi, total=%d, muốn 2 / 7", len(got.Records), got.Total)
	}
	s := got.Stats
	if s.TotalSessions != 7 || s.PresentCount != 3 || s.AbsentCount != 2 || s.LateCount != 1 || s.ExcusedCount != 1 {
		t.Fatalf("stats = %+v, muốn 7 buổi: 3 có mặt, 2 vắng, 1 muộn, 1 có phép (toàn bộ, không theo trang)", s)
	}
	if want := 4.0 / 7.0 * 100; s.AttendanceRate < want-0.01 || s.AttendanceRate > want+0.01 {
		t.Fatalf("attendance_rate = %v, muốn %v", s.AttendanceRate, want)
	}
	// Trang khác cho cùng thống kê.
	page4, err := svc.GetChildAttendance(ctx, parent.ID, student.ID, 4, 2)
	qa5Must(t, err)
	if page4.Stats != s {
		t.Fatalf("stats trang 4 = %+v, muốn bằng trang 1 %+v", page4.Stats, s)
	}
}

// ---- Q5-PG-02: upcoming_sessions sắp theo ngày toàn cục, cắt 20 ----

func TestQA5_ParentChildSchedule_UpcomingSortedAcrossClassesAndCapped(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	parent, student := f.user("pg02-parent"), f.user("pg02-student")
	qa5Must(t, f.db.Create(&model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: student.ID, Status: "active", CanViewAttendance: true}).Error)
	var classes []model.Class
	for _, name := range []string{"QA5 A", "QA5 B", "QA5 C"} {
		c := model.Class{Name: name, Status: "active"}
		qa5Must(t, f.db.Create(&c).Error)
		qa5Must(t, f.db.Create(&model.StudentClass{StudentID: student.ID, ClassID: c.ID, Status: "active"}).Error)
		classes = append(classes, c)
	}
	// 3 lớp x 10 buổi, ngày xen kẽ: mỗi lớp chiếm các ngày cách nhau 3 ngày, lệch nhau 1 ngày.
	for ci, c := range classes {
		for n := 0; n < 10; n++ {
			day := time.Now().AddDate(0, 0, 1+n*3+ci)
			qa5Must(t, f.db.Create(&model.ClassSession{ClassID: c.ID, SessionNumber: n + 1, Date: day, StartTime: "18:00", EndTime: "19:00", Status: model.SessionScheduled}).Error)
		}
	}
	svc := &ParentDashboardService{parentStudentRepo: repository.NewParentStudentRepository(f.db), scheduleRepo: repository.NewScheduleRepository(f.db)}
	got, err := svc.GetChildSchedule(ctx, parent.ID, student.ID)
	qa5Must(t, err)
	up := got.UpcomingSessions
	if len(up) != 20 {
		t.Fatalf("upcoming = %d buổi, muốn đúng 20 (cắt toàn cục)", len(up))
	}
	for i := 1; i < len(up); i++ {
		if up[i].Date.Before(up[i-1].Date) {
			t.Fatalf("upcoming không theo thứ tự ngày tại %d: %v sau %v", i, up[i].Date, up[i-1].Date)
		}
	}
	// 20 buổi sớm nhất của 30: lớp A,B,C xen kẽ ngày 1,2,3,4,5,6,...: lớp nào cũng có mặt.
	seen := map[string]bool{}
	for _, u := range up {
		seen[u.ClassName] = true
	}
	if len(seen) != 3 {
		t.Fatalf("20 buổi sớm nhất phải gồm cả 3 lớp, có %v", seen)
	}
}

// ---- Q5-PG-13: chờ chấm vs cần làm lại ----

func TestQA5_ParentAssignmentStats_SplitPendingReviewFromNeedsRetry(t *testing.T) {
	pf := newParentLinkFixture(t)
	parent, child := pf.user("p", "PARENT"), pf.user("c", "STUDENT")
	qa5Must(t, pf.db.Create(&model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: child.ID, Status: model.ParentStudentStatusActive, CanViewProgress: true}).Error)
	f := &assignmentsFx{t: t, db: pf.db, child: child}
	svc := NewParentDashboardService(repository.NewParentStudentRepository(pf.db), repository.NewUserRepository(pf.db),
		repository.NewEnrollmentRepository(pf.db), nil, nil, repository.NewSubmissionRepository(pf.db), nil)
	class := f.class("Lớp PG13", "active")
	future := time.Now().Add(48 * time.Hour)
	waiting := f.assignment(class, "waiting-teacher", &future, true)
	retry := f.assignment(class, "wrong-answer", &future, true)
	mixed := f.assignment(class, "pending-then-better-wrong", &future, true)
	done := f.assignment(class, "accepted", &future, true)
	f.assignment(class, "untouched", &future, true)
	f.submit(child.ID, waiting, model.VerdictPending, 0, 0)
	f.submit(child.ID, retry, model.VerdictWrongAnswer, 1, 4)
	// Bài nộp tốt nhất (nhiều test qua nhất) là wrong_answer => cần làm lại, dù bài mới nhất còn pending.
	f.submit(child.ID, mixed, model.VerdictWrongAnswer, 3, 4)
	f.submit(child.ID, mixed, model.VerdictPending, 0, 4)
	f.submit(child.ID, done, model.VerdictAccepted, 4, 4)

	got, err := svc.GetChildAssignments(context.Background(), parent.ID, child.ID, 1, 20)
	qa5Must(t, err)
	s := got.Stats
	if s.InProgress != 3 || s.PendingReview != 1 || s.NeedsRetry != 2 || s.PendingReview+s.NeedsRetry != s.InProgress {
		t.Fatalf("stats = %+v, muốn in_progress=3 = pending_review 1 + needs_retry 2", s)
	}
}

// ---- C1 / S5-02: my_submission ----

func TestQA5_AssignmentList_MySubmissionLatestAndGrade(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	me, other, teacher := f.user("s502-me"), f.user("s502-other"), f.user("s502-teacher")
	af := &assignmentsFx{t: t, db: f.db, child: me}
	class := af.class("QA5 S502", "active")
	future := time.Now().Add(48 * time.Hour)
	untouched := af.assignment(class, "untouched", &future, true)
	submitted := af.assignment(class, "submitted-no-grade", &future, true)
	graded := af.assignment(class, "graded", &future, true)
	onlyOther := af.assignment(class, "only-other-submitted", &future, true)
	af.submit(me.ID, submitted, model.VerdictPending, 0, 0)
	af.submit(me.ID, graded, model.VerdictWrongAnswer, 1, 4)
	time.Sleep(10 * time.Millisecond)
	af.submit(me.ID, graded, model.VerdictAccepted, 4, 4)
	af.submit(other.ID, onlyOther, model.VerdictAccepted, 4, 4)
	qa5Must(t, f.db.Create(&model.Grade{StudentID: me.ID, ClassID: class.ID, AssignmentID: &graded.ID, GradeType: model.GradeAssignment, Title: "g",
		Score: decimal.NewFromFloat(9), MaxScore: decimal.NewFromInt(10), Weight: decimal.NewFromInt(1), GradedBy: teacher.ID, GradedAt: time.Now()}).Error)

	svc := &AssignmentService{submissionRepo: repository.NewSubmissionRepository(f.db)}
	data, err := svc.toResponseDTOsWithMySubmission(ctx, me.ID, []model.Assignment{untouched, submitted, graded, onlyOther})
	qa5Must(t, err)
	by := map[string]*dto.MySubmissionDTO{}
	for _, d := range data {
		by[d.Title] = d.MySubmission
	}
	if by["untouched"] != nil || by["only-other-submitted"] != nil {
		t.Fatalf("bài chưa nộp (kể cả bài chỉ người khác nộp) phải có my_submission null: %+v", by)
	}
	if s := by["submitted-no-grade"]; s == nil || s.Status != "submitted" || s.Score != nil || s.MaxScore != nil || s.SubmittedAt == "" {
		t.Fatalf("submitted-no-grade = %+v, muốn status submitted, chưa có điểm", s)
	}
	if g := by["graded"]; g == nil || g.Status != "graded" || g.Score == nil || *g.Score != 9 || g.MaxScore == nil || *g.MaxScore != 10 {
		t.Fatalf("graded = %+v, muốn status graded 9/10", g)
	}
	// Bài nộp mới nhất (accepted) là cái được trả về.
	var latest model.Submission
	qa5Must(t, f.db.Where("user_id = ? AND assignment_id = ?", me.ID, graded.ID).Order("created_at DESC").First(&latest).Error)
	if by["graded"].ID != latest.ID {
		t.Fatalf("my_submission.id = %s, muốn bài nộp mới nhất %s", by["graded"].ID, latest.ID)
	}
	// submissionRepo nil (test khác dựng service không có repo): không panic, không có my_submission.
	bare := &AssignmentService{}
	if out, err := bare.toResponseDTOsWithMySubmission(ctx, me.ID, []model.Assignment{graded}); err != nil || out[0].MySubmission != nil {
		t.Fatalf("không repo: out=%+v err=%v", out, err)
	}
}

// system_error (Judge0 không chấm được) không phải bài nộp được tính: my_submission lấy bài được tính mới nhất,
// hoặc null khi chỉ có system_error.
func TestQA5_AssignmentList_MySubmissionIgnoresSystemError(t *testing.T) {
	f := newS2Fixture(t)
	me := f.user("s502-syserr-me")
	af := &assignmentsFx{t: t, db: f.db, child: me}
	class := af.class("QA5 S502 system_error", "active")
	future := time.Now().Add(48 * time.Hour)
	counted := af.assignment(class, "counted-then-system-error", &future, true)
	only := af.assignment(class, "only-system-error", &future, true)
	af.submit(me.ID, counted, model.VerdictWrongAnswer, 1, 4)
	time.Sleep(10 * time.Millisecond)
	af.submit(me.ID, counted, VerdictJudgeUnavailable, 0, 0)
	af.submit(me.ID, only, VerdictJudgeUnavailable, 0, 0)

	svc := &AssignmentService{submissionRepo: repository.NewSubmissionRepository(f.db)}
	data, err := svc.toResponseDTOsWithMySubmission(context.Background(), me.ID, []model.Assignment{counted, only})
	qa5Must(t, err)
	by := map[string]*dto.MySubmissionDTO{}
	for _, d := range data {
		by[d.Title] = d.MySubmission
	}
	var wrong model.Submission
	qa5Must(t, f.db.Where("user_id = ? AND assignment_id = ? AND verdict = ?", me.ID, counted.ID, model.VerdictWrongAnswer).First(&wrong).Error)
	if s := by["counted-then-system-error"]; s == nil || s.ID != wrong.ID {
		t.Fatalf("my_submission = %+v, muốn bài được tính mới nhất %s (bỏ qua system_error mới hơn)", s, wrong.ID)
	}
	if by["only-system-error"] != nil {
		t.Fatalf("bài chỉ có system_error phải có my_submission null: %+v", by["only-system-error"])
	}
}

// ---- C5 / Q5-T14: cảnh báo trùng lịch của giảng viên ----

func TestQA5_Schedule_OverlapWarningsForTeacherOtherActiveClasses(t *testing.T) {
	e := newS4ClassEnv(t)
	ctx := context.Background()
	svc := newS5ScheduleSvc(e)
	mk := func(name, status string) model.Class {
		c := model.Class{Name: name, Status: status}
		qa5Must(t, e.f.db.Create(&c).Error)
		qa5Must(t, e.f.db.Create(&model.TeacherClass{TeacherID: e.coTeacher.ID, ClassID: c.ID, Role: "primary"}).Error)
		return c
	}
	sched := func(c model.Class, day int, start, end string, active bool, until *time.Time) {
		s := model.ClassSchedule{ClassID: c.ID, DayOfWeek: day, StartTime: model.TimeOfDay(start), EndTime: model.TimeOfDay(end), IsActive: true, EffectiveFrom: time.Now().AddDate(0, 0, -30), EffectiveUntil: until}
		qa5Must(t, e.f.db.Create(&s).Error)
		if !active {
			qa5Must(t, e.f.db.Model(&s).Update("is_active", false).Error)
		}
	}
	clash := mk("QA5 Lớp trùng", "active")
	sched(clash, 1, "18:30", "19:30", true, nil)
	adjacent := mk("QA5 Lớp liền kề", "active")
	sched(adjacent, 1, "19:00", "20:00", true, nil) // bắt đầu đúng lúc lịch mới kết thúc: không trùng
	otherDay := mk("QA5 Lớp ngày khác", "active")
	sched(otherDay, 2, "18:00", "19:00", true, nil)
	archived := mk("QA5 Lớp lưu trữ", "archived")
	sched(archived, 1, "18:00", "19:00", true, nil)
	inactive := mk("QA5 Lịch tắt", "active")
	sched(inactive, 1, "18:00", "19:00", false, nil)
	past := time.Now().AddDate(0, 0, -10)
	ended := mk("QA5 Lịch hết hiệu lực", "active")
	sched(ended, 1, "18:00", "19:00", true, &past)
	strangerClass := model.Class{Name: "QA5 Giảng viên khác", Status: "active"}
	qa5Must(t, e.f.db.Create(&strangerClass).Error)
	sched(strangerClass, 1, "18:00", "19:00", true, nil)

	created, err := svc.CreateSchedule(ctx, e.class.ID, e.owner.ID, false, dto.CreateClassScheduleDTO{
		DayOfWeek: 1, StartTime: "18:00", EndTime: "19:00", EffectiveFrom: time.Now().Format("2006-01-02"),
	})
	if err != nil {
		t.Fatalf("CreateSchedule: %v (cảnh báo không bao giờ chặn)", err)
	}
	if len(created.Warnings) != 1 {
		t.Fatalf("warnings = %+v, muốn đúng 1 (lớp trùng)", created.Warnings)
	}
	w := created.Warnings[0]
	if w.ClassID != clash.ID || w.ClassName != clash.Name || w.Weekday != 1 || w.StartTime != "18:30" || w.EndTime != "19:30" {
		t.Fatalf("warning = %+v", w)
	}

	// Sửa lịch sang giờ không trùng: warnings là mảng rỗng (không phải null).
	start, end := "21:00", "22:00"
	updated, err := svc.UpdateSchedule(ctx, created.ID, e.owner.ID, false, dto.UpdateClassScheduleDTO{StartTime: &start, EndTime: &end})
	qa5Must(t, err)
	if updated.Warnings == nil || len(updated.Warnings) != 0 {
		t.Fatalf("sau khi sửa sang giờ trống: warnings = %#v, muốn mảng rỗng", updated.Warnings)
	}
	// Sửa lại vào giờ trùng: cảnh báo trở lại, vẫn lưu.
	start, end = "19:00", "19:45"
	updated, err = svc.UpdateSchedule(ctx, created.ID, e.owner.ID, false, dto.UpdateClassScheduleDTO{StartTime: &start, EndTime: &end})
	qa5Must(t, err)
	if len(updated.Warnings) != 2 {
		// 19:00-19:45 giao cả "Lớp trùng" (18:30-19:30) lẫn "Lớp liền kề" (19:00-20:00).
		t.Fatalf("warnings = %+v, muốn 2 (Lớp trùng + Lớp liền kề)", updated.Warnings)
	}
}

// ---- Q5-T07: nhắc nhở theo giọng giảng viên ----

func TestQA5_LessonReminder_TeacherGetsTeacherVoice(t *testing.T) {
	f := newS2Fixture(t)
	teacher, student := f.user("t07-teacher"), f.user("t07-student")
	class := model.Class{Name: "QA5 T07", Status: "active"}
	qa5Must(t, f.db.Create(&class).Error)
	qa5Must(t, f.db.Create(&model.TeacherClass{TeacherID: teacher.ID, ClassID: class.ID}).Error)
	qa5Must(t, f.db.Create(&model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active"}).Error)
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	start, end := now.Add(15*time.Minute), now.Add(75*time.Minute)
	qa5Must(t, f.db.Create(&model.LivestreamSession{HostID: teacher.ID, ClassID: class.ID, Title: "Unit 9", RoomName: uuid.NewString(),
		Status: model.LivestreamStatusScheduled, ScheduledAt: &start, ScheduledEndAt: &end, MaxViewers: 30}).Error)
	svc := NewLessonReminderService(f.db, nil)
	qa5Must(t, svc.Sweep(context.Background(), now))

	content := func(userID uuid.UUID) string {
		var n model.Notification
		qa5Must(t, f.db.Where("user_id = ? AND notification_type = ?", userID, "lesson_reminder").First(&n).Error)
		return n.Content
	}
	if got := content(teacher.ID); strings.Contains(got, "Chuẩn bị vào lớp") || !strings.Contains(got, "Buổi dạy sắp bắt đầu") {
		t.Fatalf("giảng viên nhận %q, muốn giọng người dạy", got)
	}
	if got := content(student.ID); got != "Unit 9 · 10:15 · 15/10. Chuẩn bị vào lớp nhé." {
		t.Fatalf("học viên nhận %q, muốn giữ nguyên giọng học viên", got)
	}
	// Nhắc đúng giờ (phase 0).
	qa5Must(t, svc.Sweep(context.Background(), start))
	var starts []model.Notification
	qa5Must(t, f.db.Where("user_id = ? AND title = ?", teacher.ID, "Đến giờ vào học").Find(&starts).Error)
	if len(starts) != 1 || !strings.HasPrefix(starts[0].Content, "Buổi dạy Unit 9 bắt đầu lúc") {
		t.Fatalf("nhắc đúng giờ cho giảng viên: %+v", starts)
	}
}

// ---- C4 / QA5-A02: tra tên đối tượng nhật ký (kể cả đã xoá mềm) ----

func TestQA5_AuditTargetName_ResolvesIncludingSoftDeleted(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	svc := NewAuditLogService(repository.NewAuditLogRepository(f.db))
	user := f.user("a02-user")
	qa5Must(t, f.db.Model(&model.User{}).Where("id = ?", user.ID).Update("full_name", "Đặng Bá Quân").Error)
	tag := model.Tag{Name: "qa5-" + uuid.NewString()[:6]}
	qa5Must(t, f.db.Create(&tag).Error)
	cat := model.Category{Name: "QA5 Danh mục " + uuid.NewString()[:4]}
	qa5Must(t, f.db.Create(&cat).Error)
	course := f.course(user)

	for _, tc := range []struct{ typ, id, want string }{
		{"user", user.ID.String(), "Đặng Bá Quân"},
		{"tag", tag.ID.String(), tag.Name},
		{"category", cat.ID.String(), cat.Name},
		{"course", course.ID.String(), course.Title},
	} {
		got, err := svc.AuditTargetName(ctx, tc.typ, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q err=%v, muốn %q", tc.typ, got, err, tc.want)
		}
	}
	// Xoá mềm xong vẫn tra được tên (dòng "xoá" của nhật ký cần).
	qa5Must(t, f.db.Delete(&cat).Error)
	if got, _ := svc.AuditTargetName(ctx, "category", cat.ID.String()); got != cat.Name {
		t.Errorf("danh mục đã xoá mềm: %q, muốn %q", got, cat.Name)
	}
	// Không biết tên: "" không lỗi (id không phải UUID, loại chưa hỗ trợ, id không tồn tại).
	for _, tc := range []struct{ typ, id string }{{"user", "abc-123"}, {"setting", uuid.NewString()}, {"tag", uuid.NewString()}} {
		if got, err := svc.AuditTargetName(ctx, tc.typ, tc.id); got != "" || err != nil {
			t.Errorf("%v: got %q err=%v, muốn rỗng", tc, got, err)
		}
	}
}
