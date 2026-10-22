package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type lessonCodeAccessStub struct {
	LessonContentServiceInterface
	exerciseID uuid.UUID
	denied     bool
}

func (s lessonCodeAccessStub) GetContentsByLessonID(context.Context, uuid.UUID, uuid.UUID, bool) ([]dto.LessonContentResponseDTO, error) {
	if s.denied {
		return nil, ErrLessonLocked
	}
	return []dto.LessonContentResponseDTO{{Type: "exercise", ExerciseID: &s.exerciseID}}, nil
}
func TestLessonCodeScopedGradingAndIdempotence(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	user := f.user("lesson-code")
	exercise := model.CourseExercise{Title: "Sum", Description: "Read two integers", Language: pq.StringArray{"python"}, TimeLimit: 2, MemoryLimit: 128, PassPercentage: 100}
	if err := f.db.Create(&exercise).Error; err != nil {
		t.Fatal(err)
	}
	cases := []model.ExerciseTestCase{{ExerciseID: exercise.ID, Input: "2 3", ExpectedOutput: "5", DisplayOrder: 0}, {ExerciseID: exercise.ID, Input: "secret-input", ExpectedOutput: "secret-answer", IsHidden: true, DisplayOrder: 1}}
	if err := f.db.Create(&cases).Error; err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req dto.Judge0SubmissionDTO
		_ = json.NewDecoder(r.Body).Decode(&req)
		code, _ := base64.StdEncoding.DecodeString(req.SourceCode)
		if string(code) == "unavailable" {
			w.WriteHeader(503)
			return
		}
		status := 3
		if string(code) == "wrong" {
			status = 4
		}
		_ = json.NewEncoder(w).Encode(dto.Judge0ResponseDTO{Status: dto.Judge0StatusDTO{ID: status}, Stdout: req.ExpectedOutput})
	}))
	defer server.Close()
	s := NewExerciseService(repository.NewExerciseRepository(f.db), nil, nil)
	s.SetLessonJudge(NewHTTPJudge0Client(server.URL), lessonCodeAccessStub{exerciseID: exercise.ID})
	lesson := uuid.New()
	detail, err := s.LessonCode(ctx, lesson, exercise.ID, user, false)
	if err != nil || len(detail.Exercise.TestCases) != 1 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	req := dto.ContestCodeRequest{RequestID: uuid.New(), Language: "python", Code: "correct"}
	result, err := s.JudgeLessonCode(ctx, lesson, exercise.ID, user, false, req, true)
	if err != nil || result.Score != 100 || calls.Load() != 2 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, calls.Load())
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "secret-") {
		t.Fatalf("hidden data leaked: %s", raw)
	}
	if _, err = s.JudgeLessonCode(ctx, lesson, exercise.ID, user, false, req, true); err != nil || calls.Load() != 2 {
		t.Fatalf("retry rejudged: %v %d", err, calls.Load())
	}
	req.Code = "other"
	if _, err = s.JudgeLessonCode(ctx, lesson, exercise.ID, user, false, req, true); err == nil {
		t.Fatal("request id reused for different code")
	}
	req.RequestID = uuid.New()
	req.Code = "unavailable"
	if _, err = s.JudgeLessonCode(ctx, lesson, exercise.ID, user, false, req, true); err == nil {
		t.Fatal("infrastructure failure must not grade")
	}
	var count int64
	f.db.Model(&model.ExerciseSubmission{}).Where("exercise_id = ?", exercise.ID).Count(&count)
	if count != 1 {
		t.Fatalf("saved %d submissions", count)
	}
	detail, err = s.LessonCode(ctx, lesson, exercise.ID, user, false)
	if err != nil || detail.LastSubmission == nil || detail.LastSubmission.Score != 100 {
		t.Fatalf("persisted result: %+v %v", detail, err)
	}
	before := calls.Load()
	if _, err = s.JudgeLessonCode(ctx, lesson, uuid.New(), user, false, req, true); err == nil {
		t.Fatal("unlinked exercise accepted")
	}
	s.lessonAccess = lessonCodeAccessStub{denied: true}
	if _, err = s.JudgeLessonCode(ctx, lesson, exercise.ID, user, false, req, true); err == nil {
		t.Fatal("locked lesson accepted")
	}
	if calls.Load() != before {
		t.Fatal("unauthorized request reached Judge")
	}
}

// Gap #3: four tests of an infinite loop used to overrun the fixed 60 s grading context and come
// back as JUDGE_UNAVAILABLE with nothing stored. Judge0 now stops each run at the wall limit and
// answers TIME_LIMIT, which is stored. A judge that does not answer within the budget stays
// JUDGE_UNAVAILABLE and stores nothing.
func TestLessonCodeSubmit_TimeLimitStoredStalledJudgeUnavailable(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	user := f.user("lesson-slow")
	exercise := model.CourseExercise{Title: "Loop", Description: "Never ends", Language: pq.StringArray{"python"}, TimeLimit: 2, MemoryLimit: 128, PassPercentage: 100}
	if err := f.db.Create(&exercise).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		tc := model.ExerciseTestCase{ExerciseID: exercise.ID, Input: "x", ExpectedOutput: "x", IsHidden: i > 1, DisplayOrder: i}
		if err := f.db.Create(&tc).Error; err != nil {
			t.Fatal(err)
		}
	}
	useShortJudgeTestBudget(t, 100*time.Millisecond)
	judge := &stallingJudge{status: 5}
	s := NewExerciseService(repository.NewExerciseRepository(f.db), nil, nil)
	s.SetLessonJudge(judge, lessonCodeAccessStub{exerciseID: exercise.ID})
	lesson := uuid.New()
	req := dto.ContestCodeRequest{RequestID: uuid.New(), Language: "python", Code: "while True: pass"}
	result, err := s.JudgeLessonCode(ctx, lesson, exercise.ID, user, false, req, true)
	if err != nil || result.Status != "TIME_LIMIT" || result.Passed != 0 || result.Total != 4 || judge.calls != 4 {
		t.Fatalf("result=%+v err=%v calls=%d, want TIME_LIMIT on all 4 tests", result, err, judge.calls)
	}
	var stored model.ExerciseSubmission
	if err := f.db.First(&stored, "id = ?", req.RequestID).Error; err != nil || stored.Verdict != "time_limit" {
		t.Fatalf("stored=%+v err=%v, want verdict time_limit", stored, err)
	}

	judge.status, judge.stall = 0, 2*time.Second
	stalled := dto.ContestCodeRequest{RequestID: uuid.New(), Language: "python", Code: "print(1)"}
	if _, err := s.JudgeLessonCode(ctx, lesson, exercise.ID, user, false, stalled, true); !errors.Is(err, ErrJudgeUnavailable) {
		t.Fatalf("stalled judge: err=%v, want ErrJudgeUnavailable", err)
	}
	var count int64
	f.db.Model(&model.ExerciseSubmission{}).Where("id = ?", stalled.RequestID).Count(&count)
	if count != 0 {
		t.Fatal("a judge that never answered still stored a submission")
	}
}
