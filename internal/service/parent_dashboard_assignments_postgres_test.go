package service

// QA3 P-01: tab "Bài tập" của phụ huynh dựng danh sách từ BÀI NỘP của con, nên bài quá hạn chưa từng nộp biến mất
// và thống kê "Quá hạn" luôn 0. Test chạy SQL THẬT (schema tạm): danh sách = bài tập đã xuất bản của các lớp con đang
// theo học, LEFT JOIN bài nộp của con; phân trang theo bài tập; thống kê đếm trên toàn bộ.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type assignmentsFx struct {
	t     *testing.T
	db    *gorm.DB
	child model.User
}

func (f *assignmentsFx) class(name, enrolStatus string) model.Class {
	f.t.Helper()
	c := model.Class{Name: name + " " + uuid.NewString()[:6], Status: "active"}
	if err := f.db.Create(&c).Error; err != nil {
		f.t.Fatalf("tạo lớp: %v", err)
	}
	if enrolStatus != "" {
		if err := f.db.Create(&model.StudentClass{StudentID: f.child.ID, ClassID: c.ID, Status: enrolStatus}).Error; err != nil {
			f.t.Fatalf("ghi danh: %v", err)
		}
	}
	return c
}

func (f *assignmentsFx) assignment(class model.Class, title string, end *time.Time, published bool) model.Assignment {
	f.t.Helper()
	a := model.Assignment{
		Title: title, Description: "d", Language: pq.StringArray{"python"}, Type: "homework",
		ClassID: &class.ID, IsPublished: published, EndTime: end,
	}
	if err := f.db.Create(&a).Error; err != nil {
		f.t.Fatalf("tạo bài tập %s: %v", title, err)
	}
	if !published { // default:false của GORM bỏ qua giá trị zero khi Create
		f.db.Model(&a).Update("is_published", false)
	}
	return a
}

func (f *assignmentsFx) submit(userID uuid.UUID, a model.Assignment, verdict model.SubmissionVerdict, passed, total int) {
	f.t.Helper()
	s := model.Submission{AssignmentID: a.ID, UserID: userID, Language: "python", Code: "x", Verdict: verdict, TestCasesPassed: passed, TotalTestCases: total}
	if err := f.db.Create(&s).Error; err != nil {
		f.t.Fatalf("tạo bài nộp: %v", err)
	}
}

func TestParentDashboard_ChildAssignments_ListsUnsubmittedAndOverdue(t *testing.T) {
	pf := newParentLinkFixture(t)
	parent, child, other := pf.user("p", "PARENT"), pf.user("c", "STUDENT"), pf.user("o", "STUDENT")
	if err := pf.db.Create(&model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: child.ID, Status: model.ParentStudentStatusActive, CanViewProgress: true}).Error; err != nil {
		t.Fatal(err)
	}
	f := &assignmentsFx{t: t, db: pf.db, child: child}
	svc := NewParentDashboardService(repository.NewParentStudentRepository(pf.db), repository.NewUserRepository(pf.db),
		repository.NewEnrollmentRepository(pf.db), nil, nil, repository.NewSubmissionRepository(pf.db), nil)

	now := time.Now()
	past, future := now.Add(-48*time.Hour), now.Add(48*time.Hour)
	active := f.class("Lớp đang học", "active")
	dropped := f.class("Lớp đã rời", "dropped")
	notMember := f.class("Lớp không tham gia", "")

	overdue := f.assignment(active, "overdue-unsubmitted", &past, true)
	f.assignment(active, "future-unsubmitted", &future, true)
	f.assignment(active, "no-deadline-unsubmitted", nil, true)
	inProgress := f.assignment(active, "past-submitted-wrong", &past, true)
	done := f.assignment(active, "accepted", &future, true)
	f.assignment(active, "draft-past-due", &past, false) // chưa xuất bản: học sinh cũng không thấy
	f.assignment(dropped, "dropped-class", &past, true)
	f.assignment(notMember, "foreign-class", &past, true)

	f.submit(child.ID, inProgress, model.VerdictWrongAnswer, 1, 4)
	f.submit(child.ID, done, model.VerdictWrongAnswer, 2, 4)
	f.submit(child.ID, done, model.VerdictAccepted, 4, 4)
	f.submit(other.ID, overdue, model.VerdictAccepted, 4, 4) // bài nộp của HỌC SINH KHÁC không làm bài của con hết hạn

	ctx := context.Background()
	got, err := svc.GetChildAssignments(ctx, parent.ID, child.ID, 1, 20)
	if err != nil {
		t.Fatalf("GetChildAssignments: %v", err)
	}

	want := map[string]string{
		"overdue-unsubmitted": "overdue", "future-unsubmitted": "not_started", "no-deadline-unsubmitted": "not_started",
		"past-submitted-wrong": "in_progress", "accepted": "completed",
	}
	gotStatus := map[string]string{}
	for _, a := range got.Assignments {
		gotStatus[a.Title] = a.Status
	}
	if len(gotStatus) != len(want) || got.Total != int64(len(want)) {
		t.Fatalf("danh sách = %v (total=%d), muốn đúng %v", gotStatus, got.Total, want)
	}
	for title, st := range want {
		if gotStatus[title] != st {
			t.Errorf("%s: status %q, muốn %q", title, gotStatus[title], st)
		}
	}
	wantStats := struct{ total, completed, inProgress, notStarted, overdue int }{5, 1, 1, 2, 1}
	s := got.Stats
	if s.TotalAssignments != wantStats.total || s.Completed != wantStats.completed || s.InProgress != wantStats.inProgress ||
		s.NotStarted != wantStats.notStarted || s.Overdue != wantStats.overdue {
		t.Errorf("stats = %+v, muốn %+v (Overdue phải được điền)", s, wantStats)
	}
	for _, a := range got.Assignments {
		if a.Title == "accepted" && (a.SubmissionCount != 2 || a.BestVerdict == nil || *a.BestVerdict != "accepted" || a.TestCasesPassed != 4 || a.LastSubmittedAt == nil) {
			t.Errorf("tổng hợp bài nộp của 'accepted' sai: %+v", a)
		}
		if a.Title == "overdue-unsubmitted" && (a.SubmissionCount != 0 || a.BestVerdict != nil || a.LastSubmittedAt != nil || a.DueDate == nil) {
			t.Errorf("bài chưa nộp không được có dữ liệu nộp: %+v", a)
		}
		if a.ClassName == "" {
			t.Errorf("%s thiếu tên lớp", a.Title)
		}
	}

	// Phân trang theo BÀI TẬP: 3 trang cỡ 2 phủ đúng 5 bài, không trùng, không sót, stats vẫn là của toàn bộ.
	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		res, err := svc.GetChildAssignments(ctx, parent.ID, child.ID, page, 2)
		if err != nil {
			t.Fatal(err)
		}
		if res.Total != 5 || res.Stats.TotalAssignments != 5 || res.Stats.Overdue != 1 {
			t.Errorf("trang %d: total=%d stats=%+v, muốn total=5 và stats toàn bộ", page, res.Total, res.Stats)
		}
		for _, a := range res.Assignments {
			if seen[a.Title] {
				t.Errorf("bài %s xuất hiện ở hai trang", a.Title)
			}
			seen[a.Title] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("phân trang chỉ phủ %d/5 bài: %v", len(seen), seen)
	}
}

// Con chưa vào lớp nào: danh sách rỗng (không phải lỗi) và mọi bộ đếm bằng 0.
func TestParentDashboard_ChildAssignments_NoClasses(t *testing.T) {
	pf := newParentLinkFixture(t)
	parent, child := pf.user("p", "PARENT"), pf.user("c", "STUDENT")
	if err := pf.db.Create(&model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: child.ID, Status: model.ParentStudentStatusActive, CanViewProgress: true}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewParentDashboardService(repository.NewParentStudentRepository(pf.db), repository.NewUserRepository(pf.db),
		repository.NewEnrollmentRepository(pf.db), nil, nil, repository.NewSubmissionRepository(pf.db), nil)
	got, err := svc.GetChildAssignments(context.Background(), parent.ID, child.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Assignments) != 0 || got.Total != 0 || got.Stats.TotalAssignments != 0 || got.Stats.Overdue != 0 {
		t.Errorf("muốn rỗng, nhận %+v", got)
	}
}
