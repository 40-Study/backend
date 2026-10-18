package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
