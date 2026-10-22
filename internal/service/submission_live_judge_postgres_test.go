package service

// Gap #5 (QA 261021), service side. Live-assignment Submit stays asynchronous (the row is created
// pending, grading runs in the background), but a Judge0 outage no longer stores a wrong verdict:
// it used to store runtime_error, now the row is marked VerdictJudgeUnavailable ("system_error")
// so the UI can ask the student to submit again. Run and run-custom return ErrJudgeUnavailable
// instead of a 200 echoing err.Error() with the Judge0 URL.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

// submitAndWait submits, checks the immediate answer is the pending row, then waits for the
// background grading to finish.
func (e *liveJudgeEnv) submitAndWait(t *testing.T, code string) model.Submission {
	t.Helper()
	sub, err := e.svc.Submit(context.Background(), false, dto.CreateSubmissionDTO{AssignmentID: e.assignment.ID.String(), UserID: e.student.ID.String(), Language: "python", Code: code})
	if err != nil || sub == nil || sub.Verdict != model.VerdictPending {
		t.Fatalf("Submit must answer at once with the pending row: %+v err=%v", sub, err)
	}
	return waitForGradedSubmission(t, e.f.db, sub.ID)
}

// waitForGradedSubmission polls until background grading has replaced the pending verdict.
func waitForGradedSubmission(t *testing.T, db *gorm.DB, id uuid.UUID) model.Submission {
	t.Helper()
	var row model.Submission
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if err := db.First(&row, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if row.Verdict != model.VerdictPending {
			return row
		}
	}
	t.Fatalf("submission %s still pending after 5 s", id)
	return row
}

func TestLiveAssignmentSubmit_JudgeOutageMarksRowJudgeUnavailable(t *testing.T) {
	leak := errors.New(`Post "http://localhost:2358/submissions?base64_encoded=true&wait=true": dial tcp [::1]:2358: connect: connection refused`)
	e := newLiveJudgeEnv(t, &stallingJudge{fail: leak})
	row := e.submitAndWait(t, "print(input())")
	if row.Verdict != VerdictJudgeUnavailable || row.TestCasesPassed != 0 {
		t.Fatalf("outage stored verdict=%s passed=%d, want %s (not a verdict on the code)", row.Verdict, row.TestCasesPassed, VerdictJudgeUnavailable)
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

func TestLiveAssignmentSubmit_StalledJudgeIsUnavailableTimeLimitIsAVerdict(t *testing.T) {
	useShortJudgeTestBudget(t, 100*time.Millisecond)
	judge := &stallingJudge{status: 3}
	e := newLiveJudgeEnv(t, judge)
	if row := e.submitAndWait(t, "print(input())"); row.Verdict != model.VerdictAccepted || row.TestCasesPassed != 2 || row.TotalTestCases != 2 {
		t.Fatalf("accepted submit stored %+v", row)
	}
	if judge.lastReq.CPUTimeLimit != 2 {
		t.Fatalf("assignment time limit not passed to Judge0: %+v", judge.lastReq)
	}
	judge.status = 5
	if row := e.submitAndWait(t, "import time\ntime.sleep(60)"); row.Verdict != model.VerdictTimeLimitExceed {
		t.Fatalf("Judge0 TIME_LIMIT stored as %s", row.Verdict)
	}
	judge.status, judge.stall = 0, 2*time.Second
	if row := e.submitAndWait(t, "print(1)"); row.Verdict != VerdictJudgeUnavailable {
		t.Fatalf("judge that never answered stored as %s, want %s", row.Verdict, VerdictJudgeUnavailable)
	}
}
