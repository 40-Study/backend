package repository

// Kết quả từng test case của bài nộp (contract sec. 6 và 7) — Postgres THẬT, chạy trong 1 transaction rồi ROLLBACK.
// SaveGradingOutcome ghi kết quả + verdict tổng trong MỘT transaction thay cho lần chấm trước (redelivery không nhân
// đôi dòng); test_case_id không có FK (lịch sử còn khi ca bị xoá); xoá bài nộp thì kết quả xoá theo (cascade);
// bài nộp không tồn tại -> gorm.ErrRecordNotFound và không ghi gì.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type gradingFixture struct {
	tx       *gorm.DB
	repo     *ExerciseSubmissionResultRepository
	exercise model.CourseExercise
	cases    []model.ExerciseTestCase
	sub      model.ExerciseSubmission
}

func newGradingFixture(t *testing.T) *gradingFixture {
	t.Helper()
	db := openTestPostgres(t)
	for _, m := range []any{&model.ExerciseSubmissionResult{}, &model.CodeDraft{}} {
		if !db.Migrator().HasTable(m) {
			t.Fatalf("DB chưa có bảng %T: chạy migrate (go run ./cmd/seed -mode=base hoặc scripts/run-lane-tests.sh) trước", m)
		}
	}
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })

	s := uuid.NewString()
	user := model.User{Email: "grading-" + s + "@40study.test", PasswordHash: "x", UserName: "grading-" + s}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatalf("tạo user: %v", err)
	}
	f := &gradingFixture{tx: tx, repo: NewExerciseSubmissionResultRepository(tx)}
	f.exercise = model.CourseExercise{Title: "Tổng hai số", Description: "d", Language: []string{"python"}}
	if err := tx.Create(&f.exercise).Error; err != nil {
		t.Fatalf("tạo exercise: %v", err)
	}
	for i := 0; i < 2; i++ {
		tc := model.ExerciseTestCase{ExerciseID: f.exercise.ID, Input: "1 2", ExpectedOutput: "3", DisplayOrder: i, IsHidden: i == 1}
		if err := tx.Create(&tc).Error; err != nil {
			t.Fatalf("tạo test case: %v", err)
		}
		f.cases = append(f.cases, tc)
	}
	f.sub = model.ExerciseSubmission{ExerciseID: f.exercise.ID, UserID: user.ID, Language: "python", Code: "x",
		Verdict: string(model.VerdictRunning), TotalTestCases: 2}
	if err := tx.Create(&f.sub).Error; err != nil {
		t.Fatalf("tạo submission: %v", err)
	}
	return f
}

func (f *gradingFixture) rows(verdicts ...model.SubmissionVerdict) []model.ExerciseSubmissionResult {
	out := make([]model.ExerciseSubmissionResult, 0, len(verdicts))
	for i, v := range verdicts {
		tcID := f.cases[i%len(f.cases)].ID
		ms := 10 + i
		out = append(out, model.ExerciseSubmissionResult{TestCaseID: &tcID, DisplayOrder: len(verdicts) - 1 - i,
			IsHidden: i == 1, Verdict: v, ActualOutput: "3", ExecutionTimeMs: &ms})
	}
	return out
}

func (f *gradingFixture) reloadSub(t *testing.T) model.ExerciseSubmission {
	t.Helper()
	var s model.ExerciseSubmission
	if err := f.tx.First(&s, "id = ?", f.sub.ID).Error; err != nil {
		t.Fatalf("đọc lại submission: %v", err)
	}
	return s
}

func TestSaveGradingOutcome_PG_ReplacesEarlierRowsOnRedelivery(t *testing.T) {
	f := newGradingFixture(t)
	ctx := context.Background()

	f.sub.Verdict = string(model.VerdictMemoryLimitExceed) // 21 ký tự: không vừa varchar(20) cũ
	f.sub.TestCasesPassed = 1
	if err := f.repo.SaveGradingOutcome(ctx, &f.sub, f.rows(model.VerdictAccepted, model.VerdictMemoryLimitExceed)); err != nil {
		t.Fatalf("lần chấm 1: %v", err)
	}
	got, err := f.repo.ListBySubmission(ctx, f.sub.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("lần 1: %d dòng, %v; muốn 2", len(got), err)
	}
	if got[0].DisplayOrder != 0 || got[1].DisplayOrder != 1 {
		t.Errorf("ListBySubmission phải theo display_order tăng dần: %d, %d", got[0].DisplayOrder, got[1].DisplayOrder)
	}
	if s := f.reloadSub(t); s.Verdict != string(model.VerdictMemoryLimitExceed) || s.TestCasesPassed != 1 {
		t.Errorf("verdict tổng = %q passed=%d", s.Verdict, s.TestCasesPassed)
	}

	// Redelivery: chấm lại thay thế toàn bộ, không nhân đôi.
	f.sub.Verdict = string(model.VerdictAccepted)
	f.sub.TestCasesPassed = 2
	if err := f.repo.SaveGradingOutcome(ctx, &f.sub, f.rows(model.VerdictAccepted)); err != nil {
		t.Fatalf("lần chấm 2: %v", err)
	}
	got, _ = f.repo.ListBySubmission(ctx, f.sub.ID)
	if len(got) != 1 || got[0].Verdict != model.VerdictAccepted || got[0].SubmissionID != f.sub.ID {
		t.Fatalf("sau redelivery: %+v; muốn đúng 1 dòng accepted", got)
	}
	if s := f.reloadSub(t); s.Verdict != string(model.VerdictAccepted) || s.TestCasesPassed != 2 {
		t.Errorf("verdict tổng sau lần 2 = %q passed=%d", s.Verdict, s.TestCasesPassed)
	}

	// judge_unavailable: không có dòng nào, dòng cũ bị xoá.
	f.sub.Verdict = string(model.VerdictJudgeUnavailable)
	if err := f.repo.SaveGradingOutcome(ctx, &f.sub, nil); err != nil {
		t.Fatalf("lần chấm 3: %v", err)
	}
	if got, _ = f.repo.ListBySubmission(ctx, f.sub.ID); len(got) != 0 {
		t.Errorf("judge_unavailable vẫn còn %d dòng kết quả", len(got))
	}
}

func TestSaveGradingOutcome_PG_AtomicOnRowFailure(t *testing.T) {
	f := newGradingFixture(t)
	ctx := context.Background()
	if err := f.repo.SaveGradingOutcome(ctx, &f.sub, f.rows(model.VerdictAccepted)); err != nil {
		t.Fatal(err)
	}
	before := f.reloadSub(t)

	bad := f.rows(model.VerdictAccepted, model.SubmissionVerdict(strings.Repeat("x", 40))) // vượt varchar(32)
	f.sub.Verdict = string(model.VerdictWrongAnswer)
	if err := f.repo.SaveGradingOutcome(ctx, &f.sub, bad); err == nil {
		t.Fatal("dòng vượt varchar(32) phải làm lỗi")
	}
	if s := f.reloadSub(t); s.Verdict != before.Verdict {
		t.Errorf("verdict tổng đổi thành %q dù transaction lỗi (muốn giữ %q)", s.Verdict, before.Verdict)
	}
	if got, _ := f.repo.ListBySubmission(ctx, f.sub.ID); len(got) != 1 {
		t.Errorf("dòng cũ phải còn nguyên khi lần ghi mới lỗi: %d dòng", len(got))
	}
}

func TestSaveGradingOutcome_PG_MissingSubmission(t *testing.T) {
	f := newGradingFixture(t)
	ghost := f.sub
	ghost.ID = uuid.New()
	err := f.repo.SaveGradingOutcome(context.Background(), &ghost, f.rows(model.VerdictAccepted))
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("bài nộp không tồn tại: err = %v, muốn gorm.ErrRecordNotFound", err)
	}
	var n int64
	f.tx.Model(&model.ExerciseSubmissionResult{}).Where("submission_id = ?", ghost.ID).Count(&n)
	if n != 0 {
		t.Errorf("đã ghi %d dòng cho bài nộp không tồn tại", n)
	}
}

func TestExerciseSubmissionResults_PG_HistorySurvivesCaseDeleteAndCascadesWithSubmission(t *testing.T) {
	f := newGradingFixture(t)
	ctx := context.Background()
	if err := f.repo.SaveGradingOutcome(ctx, &f.sub, f.rows(model.VerdictAccepted, model.VerdictWrongAnswer)); err != nil {
		t.Fatal(err)
	}
	// Xoá test case: kết quả cũ vẫn còn (test_case_id không có FK).
	if err := f.tx.Delete(&model.ExerciseTestCase{}, "id = ?", f.cases[0].ID).Error; err != nil {
		t.Fatalf("xoá test case bị chặn (test_case_id không được có FK): %v", err)
	}
	if got, _ := f.repo.ListBySubmission(ctx, f.sub.ID); len(got) != 2 {
		t.Fatalf("sau khi xoá test case còn %d dòng, muốn 2", len(got))
	}
	// Xoá bài nộp: kết quả xoá theo (FK cascade trên submission_id).
	if err := f.tx.Delete(&model.ExerciseSubmission{}, "id = ?", f.sub.ID).Error; err != nil {
		t.Fatalf("xoá submission: %v", err)
	}
	if got, _ := f.repo.ListBySubmission(ctx, f.sub.ID); len(got) != 0 {
		t.Errorf("xoá submission mà còn %d dòng kết quả (thiếu ON DELETE CASCADE)", len(got))
	}
}

func TestCodeDraft_PG_UniquePerUserAndScope(t *testing.T) {
	f := newGradingFixture(t)
	d := model.CodeDraft{UserID: f.sub.UserID, Scope: "sandbox:src/main.py", Language: "python", Code: "print(1)"}
	if err := f.tx.Create(&d).Error; err != nil {
		t.Fatalf("tạo draft: %v", err)
	}
	if err := f.tx.SavePoint("dup").Error; err != nil {
		t.Fatal(err)
	}
	dup := model.CodeDraft{UserID: f.sub.UserID, Scope: d.Scope}
	if err := f.tx.Create(&dup).Error; err == nil {
		t.Fatal("(user_id, scope) trùng phải bị unique index chặn")
	}
	f.tx.RollbackTo("dup")
	other := model.CodeDraft{UserID: uuid.New(), Scope: d.Scope}
	if err := f.tx.Create(&other).Error; err != nil {
		t.Fatalf("cùng scope, user khác phải được: %v", err)
	}
}
