package service

// Lane S3 (nộp bài): Submit, RunCode và RunCustomCode chỉ nhận người XEM được assignment
// (AssignmentService.CanView: đã publish và là thành viên lớp/phiên, hoặc chủ/admin). Người ngoài,
// giảng viên khác, bản nháp của người khác và id không tồn tại đều ErrAssignmentNotFound, và Judge0
// không bị gọi, không có bài nộp nào được ghi. Postgres thật; Judge0 giả đếm số lần gọi.
// Bỏ requireViewable khỏi một trong ba hàm thì test này ĐỎ.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type s3CountingJudge struct{ calls atomic.Int32 }

func (j *s3CountingJudge) Submit(context.Context, dto.Judge0SubmissionDTO) (*dto.Judge0ResponseDTO, error) {
	j.calls.Add(1)
	return nil, errors.New("judge0 giả: không chạy thật")
}
func (j *s3CountingJudge) GetResult(context.Context, string) (*dto.Judge0ResponseDTO, error) {
	j.calls.Add(1)
	return nil, errors.New("judge0 giả: không chạy thật")
}

func TestS3_Submission_ChiNguoiXemDuocAssignmentMoiNopVaChayThu(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	courseOwner, student, outsider, otherTeacher, admin := f.user("owner"), f.user("student"), f.user("outsider"), f.user("other-teacher"), f.user("admin")

	course := f.course(courseOwner)
	class := model.Class{Name: "QA-s3-sub", CourseID: &course.ID, Status: "active"}
	if err := f.db.Create(&class).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	published := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "pub", Description: "d", Language: []string{"python"}, IsPublished: true}
	draft := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "draft", Description: "d", Language: []string{"python"}}
	for _, a := range []*model.Assignment{&published, &draft} {
		if err := f.db.Create(a).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.db.Create(&model.TestCase{AssignmentID: a.ID, Input: "1", ExpectedOutput: "1"}).Error; err != nil {
			t.Fatal(err)
		}
	}

	assignmentSvc := NewAssignmentService(repository.NewAssignmentRepository(f.db), repository.NewTestCaseRepository(f.db), nil,
		repository.NewClassRepository(f.db), s3SessionGate{})
	judge := &s3CountingJudge{}
	svc := &SubmissionService{
		repo:          repository.NewSubmissionRepository(f.db),
		assignmentSvc: assignmentSvc,
		testCaseRepo:  repository.NewTestCaseRepository(f.db),
		judge0Client:  judge,
	}
	countSubmissions := func() int64 {
		var n int64
		f.db.Model(&model.Submission{}).Count(&n)
		return n
	}

	type attempt struct {
		name    string
		a       model.Assignment
		user    model.User
		isAdmin bool
		allowed bool
	}
	attempts := []attempt{
		{"người ngoài, bài đã publish", published, outsider, false, false},
		{"giảng viên khác, bài đã publish", published, otherTeacher, false, false},
		{"học viên trong lớp, bản nháp", draft, student, false, false},
		{"người ngoài, bản nháp", draft, outsider, false, false},
		{"học viên trong lớp, bài đã publish", published, student, false, true},
		{"chủ khoá, bài đã publish", published, courseOwner, false, true},
		{"admin, bài đã publish", published, admin, true, true},
	}
	run := func(kind string, at attempt, do func() error) {
		t.Helper()
		err := do()
		if at.allowed && errors.Is(err, ErrAssignmentNotFound) {
			t.Errorf("%s / %s: bị chặn nhầm (%v)", kind, at.name, err)
		}
		if !at.allowed && !errors.Is(err, ErrAssignmentNotFound) {
			t.Errorf("%s / %s: err=%v, muốn ErrAssignmentNotFound", kind, at.name, err)
		}
	}

	for _, at := range attempts {
		beforeJudge, beforeSubs := judge.calls.Load(), countSubmissions()
		run("RunCode", at, func() error {
			_, err := svc.RunCode(ctx, at.user.ID, at.isAdmin, dto.RunCodeDTO{AssignmentID: at.a.ID.String(), Language: "python", Code: "print(1)"})
			return err
		})
		run("RunCustomCode", at, func() error {
			_, err := svc.RunCustomCode(ctx, at.user.ID, at.isAdmin, dto.RunCustomCodeDTO{AssignmentID: at.a.ID.String(), Language: "python", Code: "print(1)", CustomInput: "1"})
			return err
		})
		if !at.allowed {
			if got := judge.calls.Load(); got != beforeJudge {
				t.Errorf("%s: Judge0 bị gọi %d lần dù không có quyền", at.name, got-beforeJudge)
			}
		}
		run("Submit", at, func() error {
			_, err := svc.Submit(ctx, at.isAdmin, dto.CreateSubmissionDTO{AssignmentID: at.a.ID.String(), UserID: at.user.ID.String(), Language: "python", Code: "print(1)"})
			return err
		})
		if !at.allowed && countSubmissions() != beforeSubs {
			t.Errorf("%s: bài nộp vẫn được ghi dù không có quyền", at.name)
		}
	}

	if err := func() error {
		_, err := svc.Submit(ctx, true, dto.CreateSubmissionDTO{AssignmentID: uuid.NewString(), UserID: admin.ID.String(), Language: "python", Code: "x"})
		return err
	}(); !errors.Is(err, ErrAssignmentNotFound) {
		t.Errorf("id không tồn tại: err=%v, muốn ErrAssignmentNotFound kể cả admin", err)
	}

	// Chờ các goroutine chấm bài do Submit hợp lệ khởi động xong trước khi fixture drop schema.
	time.Sleep(500 * time.Millisecond)
}
