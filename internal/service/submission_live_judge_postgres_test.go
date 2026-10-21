package service

// Gap #5 (QA 261021), service side: a live-assignment code submission is judged BEFORE any row is
// written, so a Judge0 outage returns ErrJudgeUnavailable and stores nothing (it used to store
// verdict=runtime_error from a background goroutine), while a slow program is stored as
// time_limit_exceeded. Run and run-custom return ErrJudgeUnavailable instead of err.Error().

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type liveJudgeEnv struct {
	f          *s2Fixture
	svc        *SubmissionService
	student    model.User
	assignment model.Assignment
}

func newLiveJudgeEnv(t *testing.T, judge Judge0Client) *liveJudgeEnv {
	t.Helper()
	f := newS2Fixture(t)
	owner, student := f.user("live-owner"), f.user("live-student")
	course := f.course(owner)
	class := model.Class{Name: "QA live judge", CourseID: &course.ID, Status: "active"}
	if err := f.db.Create(&class).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	assignment := model.Assignment{ClassID: &class.ID, Type: "homework", Title: "Echo", Description: "d", Language: []string{"python"}, IsPublished: true, TimeLimit: 2, MemoryLimit: 128}
	if err := f.db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []model.TestCase{{AssignmentID: assignment.ID, Input: "1", ExpectedOutput: "1"}, {AssignmentID: assignment.ID, Input: "2", ExpectedOutput: "2", IsHidden: true}} {
		if err := f.db.Create(&tc).Error; err != nil {
			t.Fatal(err)
		}
	}
	assignments := NewAssignmentService(repository.NewAssignmentRepository(f.db), repository.NewTestCaseRepository(f.db),
		repository.NewSubmissionRepository(f.db), repository.NewClassRepository(f.db), nil)
	svc := &SubmissionService{repo: repository.NewSubmissionRepository(f.db), assignmentSvc: assignments, testCaseRepo: repository.NewTestCaseRepository(f.db), judge0Client: judge}
	return &liveJudgeEnv{f: f, svc: svc, student: student, assignment: assignment}
}

func (e *liveJudgeEnv) stored(t *testing.T) []model.Submission {
	t.Helper()
	// Submit used to finish grading in a goroutine; give a regression the time to write its verdict.
	time.Sleep(200 * time.Millisecond)
	var rows []model.Submission
	if err := e.f.db.Where("assignment_id = ?", e.assignment.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func (e *liveJudgeEnv) submit(code string) (*model.Submission, error) {
	return e.svc.Submit(context.Background(), false, dto.CreateSubmissionDTO{AssignmentID: e.assignment.ID.String(), UserID: e.student.ID.String(), Language: "python", Code: code})
}

func TestLiveAssignmentSubmit_JudgeOutageStoresNothing(t *testing.T) {
	leak := errors.New(`Post "http://localhost:2358/submissions?base64_encoded=true&wait=true": dial tcp [::1]:2358: connect: connection refused`)
	e := newLiveJudgeEnv(t, &stallingJudge{fail: leak})
	if _, err := e.submit("print(input())"); !errors.Is(err, ErrJudgeUnavailable) {
		t.Fatalf("Submit during outage: err=%v, want ErrJudgeUnavailable", err)
	}
	if rows := e.stored(t); len(rows) != 0 {
		t.Fatalf("outage stored %d row(s), first verdict=%s", len(rows), rows[0].Verdict)
	}
	run, err := e.svc.RunCode(context.Background(), e.student.ID, false, dto.RunCodeDTO{AssignmentID: e.assignment.ID.String(), Language: "python", Code: "print(1)"})
	if !errors.Is(err, ErrJudgeUnavailable) || run != nil {
		t.Fatalf("RunCode during outage: result=%+v err=%v", run, err)
	}
	custom, err := e.svc.RunCustomCode(context.Background(), e.student.ID, false, dto.RunCustomCodeDTO{AssignmentID: e.assignment.ID.String(), Language: "python", Code: "print(1)", CustomInput: "1"})
	if !errors.Is(err, ErrJudgeUnavailable) || custom != nil {
		t.Fatalf("RunCustomCode during outage: result=%+v err=%v", custom, err)
	}
	if strings.Contains(err.Error(), "2358") {
		t.Fatalf("error carries the judge address: %v", err)
	}
}

func TestLiveAssignmentSubmit_GradesBeforeStoringAndSlowProgramIsTimeLimit(t *testing.T) {
	useShortJudgeTestBudget(t, 100*time.Millisecond)
	judge := &stallingJudge{status: 3}
	e := newLiveJudgeEnv(t, judge)
	sub, err := e.submit("print(input())")
	if err != nil || sub.Verdict != model.VerdictAccepted || sub.TestCasesPassed != 2 || sub.TotalTestCases != 2 {
		t.Fatalf("accepted submit: %+v err=%v", sub, err)
	}
	if judge.lastReq.CPUTimeLimit != 2 {
		t.Fatalf("assignment time limit not passed to Judge0: %+v", judge.lastReq)
	}

	judge.status, judge.stall = 0, 2*time.Second
	slow, err := e.submit("import time\ntime.sleep(60)")
	if err != nil || slow.Verdict != model.VerdictTimeLimitExceed || slow.TestCasesPassed != 0 {
		t.Fatalf("slow submit: %+v err=%v, want time_limit_exceeded", slow, err)
	}
	rows := e.stored(t)
	if len(rows) != 2 {
		t.Fatalf("stored %d rows, want 2", len(rows))
	}
	for _, row := range rows {
		if row.Verdict == model.VerdictPending || row.Verdict == model.VerdictRuntimeError {
			t.Errorf("row %s stored verdict %s", row.ID, row.Verdict)
		}
	}
}
