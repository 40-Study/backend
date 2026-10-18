package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"net/http"
	"net/http/httptest"
	"strings"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"sync/atomic"
	"testing"
	"time"
)

func TestJudgeHTTPPollingCreatesOneJobAndRunOmitsExpectedOutput(t *testing.T) {
	var posts, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			posts.Add(1)
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if _, ok := req["expected_output"]; ok {
				t.Error("run must not compare output with an empty string")
			}
			_, _ = w.Write([]byte(`{"token":"job","status":{"id":1}}`))
			return
		}
		gets.Add(1)
		_, _ = w.Write([]byte(`{"token":"job","status":{"id":3},"stdout":"NQo="}`))
	}))
	defer server.Close()
	result, err := NewHTTPJudge0Client(server.URL).Submit(context.Background(), createJudge0Request("print(5)", 71, "", "", 2, 131072))
	if err != nil || result.Status.ID != 3 || posts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("result=%+v err=%v posts=%d gets=%d", result, err, posts.Load(), gets.Load())
	}
}
func TestContestCodeGradingPersistenceAndAccess(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	owner, student, stranger := f.user("code-owner"), f.user("code-student"), f.user("code-stranger")
	repo := repository.NewContestRepository(f.db)
	s := NewContestService(repo, nil, nil, nil)
	now := time.Now()
	s.SetClock(func() time.Time { return now })
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req dto.Judge0SubmissionDTO
		_ = json.NewDecoder(r.Body).Decode(&req)
		decoded, _ := base64.StdEncoding.DecodeString(req.SourceCode)
		req.SourceCode = string(decoded)
		status := 3
		if req.SourceCode == "wrong" {
			status = 4
		}
		if req.SourceCode == "compile" {
			status = 6
		}
		if req.SourceCode == "unavailable" {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(dto.Judge0ResponseDTO{Status: dto.Judge0StatusDTO{ID: status}, Stdout: req.ExpectedOutput, Time: "0.01", Memory: 32})
	}))
	defer server.Close()
	s.SetJudge(NewHTTPJudge0Client(server.URL))
	input := dto.ContestCodeProblemInput{Title: "Sum", Description: "Read two integers", Difficulty: "EASY", Points: 100, TimeLimit: 2, MemoryLimit: 128, Tests: []dto.ContestCodeTest{{Input: "2 3", ExpectedOutput: "5"}, {Input: "hidden-input", ExpectedOutput: "hidden-answer", Hidden: true}}}
	teacher := &ContestActor{UserID: owner, ActiveRole: "TEACHER"}
	pupil := &ContestActor{UserID: student, ActiveRole: "STUDENT"}
	request := &dto.ContestUpsertRequest{Title: "Code contest", Type: "CODING", Problems: []dto.ContestCodeProblemInput{input}, StartTime: now.Add(time.Hour), EndTime: now.Add(4 * time.Hour), DurationMinutes: 60, IsPublic: true}
	created, err := s.Create(ctx, teacher, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.Type != "CODING" || len(created.Problems) != 1 || created.Quiz != nil {
		t.Fatalf("bad coding draft: %+v", created)
	}
	request.Problems[0].Title = "Edited Sum"
	edited, err := s.Update(ctx, created.ID, teacher, request)
	if err != nil || edited.Problems[0].Title != "Edited Sum" {
		t.Fatalf("update: %+v %v", edited, err)
	}
	if _, err := s.SubmitReview(ctx, created.ID, teacher); err != nil {
		t.Fatal(err)
	}
	admin := &ContestActor{UserID: f.user("code-admin"), IsAdmin: true, ActiveRole: "SYSTEM_ADMIN"}
	if _, err := s.Approve(ctx, created.ID, admin, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(ctx, created.ID, pupil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CodeSession(ctx, created.ID, pupil); !errors.Is(err, ErrContestNotActive) {
		t.Fatalf("pre-start access: %v", err)
	}
	now = now.Add(65 * time.Minute)
	if _, err := s.CodeSession(ctx, created.ID, &ContestActor{UserID: stranger, ActiveRole: "STUDENT"}); !errors.Is(err, ErrContestNotJoined) {
		t.Fatalf("stranger: %v", err)
	}
	session, err := s.StartCode(ctx, created.ID, pupil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(session)
	if strings.Contains(string(raw), "hidden-input") || strings.Contains(string(raw), "hidden-answer") {
		t.Fatal("hidden tests leaked")
	}
	firstStart := *session.StartedAt
	now = now.Add(time.Second)
	again, err := s.StartCode(ctx, created.ID, pupil)
	if err != nil || !again.StartedAt.Equal(firstStart) {
		t.Fatalf("start not idempotent: %v", err)
	}
	req := dto.ContestCodeRequest{RequestID: uuid.New(), ProblemID: session.Problems[0].ID, Language: "python", Code: "correct"}
	result, err := s.JudgeCode(ctx, created.ID, pupil, req, true)
	if err != nil || result.Score != 100 || result.Passed != 2 {
		t.Fatalf("grade=%+v err=%v", result, err)
	}
	raw, _ = json.Marshal(result)
	if strings.Contains(string(raw), "hidden-answer") {
		t.Fatal("hidden grading output leaked")
	}
	if _, err := s.JudgeCode(ctx, created.ID, pupil, req, true); err != nil || calls.Load() != 2 {
		t.Fatalf("idempotence: calls=%d err=%v", calls.Load(), err)
	}
	req.RequestID = uuid.New()
	req.Code = "wrong"
	if _, err := s.JudgeCode(ctx, created.ID, pupil, req, true); err != nil {
		t.Fatal(err)
	}
	req.RequestID = uuid.New()
	req.Code = "compile"
	before := calls.Load()
	compiled, err := s.JudgeCode(ctx, created.ID, pupil, req, true)
	if err != nil || compiled.Status != "COMPILATION_ERROR" || calls.Load()-before != 1 {
		t.Fatalf("compile requests: result=%+v err=%v calls=%d", compiled, err, calls.Load()-before)
	}
	req.RequestID = uuid.New()
	req.Code = "unavailable"
	if _, err := s.JudgeCode(ctx, created.ID, pupil, req, true); !errors.Is(err, ErrJudgeUnavailable) {
		t.Fatalf("infra failure: %v", err)
	}
	saved, err := s.CodeSession(ctx, created.ID, pupil)
	if err != nil || saved.Score != 100 || len(saved.Submissions) != 3 {
		t.Fatalf("best score/state: %+v %v", saved, err)
	}
	board, boardErr := s.Leaderboard(ctx, created.ID, pupil, 1, 20)
	if boardErr != nil || len(board.Items) != 1 || !board.Items[0].IsMe || !board.Items[0].Score.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("live coding leaderboard: %+v %v", board, boardErr)
	}
	finished, err := s.FinishCode(ctx, created.ID, pupil)
	if err != nil || finished.FinishedAt == nil || finished.CanSubmit {
		t.Fatalf("finish: %+v %v", finished, err)
	}
	req.RequestID = uuid.New()
	req.Code = "correct"
	if _, err := s.JudgeCode(ctx, created.ID, pupil, req, true); !errors.Is(err, ErrContestAlreadySubmitted) {
		t.Fatalf("submit after finish: %v", err)
	}
	if _, err := s.Join(ctx, created.ID, &ContestActor{UserID: stranger, ActiveRole: "STUDENT"}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&model.Contest{}).Where("id = ?", created.ID).Updates(map[string]interface{}{"start_time": time.Now().Add(-3 * time.Hour), "end_time": time.Now().Add(-time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	rows, total, err := repo.RankedRows(f.db, created.ID, false, 1, 20)
	if err != nil || total != 1 || len(rows) != 1 || !rows[0].Score.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("ranks=%+v total=%d err=%v", rows, total, err)
	}
}
