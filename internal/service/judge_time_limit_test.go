package service

// Gap #3 (QA 261021): a slow or infinite program on Submit must end as TIME_LIMIT, never as
// JUDGE_UNAVAILABLE. Before the fix no wall_time_limit was sent (Judge0 used its 20 s default) and
// 4 tests x 20 s overran the fixed 60 s grading context. Judge0 now stops the program at the wall
// limit and answers status 5 itself; the grading deadline grows with the test count (capped at
// 90 s for request grading). Our own deadline firing means the judge did not answer: that stays
// JUDGE_UNAVAILABLE, like a transport failure, a 5xx or a judge-internal status.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
)

// useShortJudgeTestBudget shrinks the per-test budget so a judge that never answers costs
// milliseconds instead of the real wall limit plus overhead.
func useShortJudgeTestBudget(t *testing.T, d time.Duration) {
	t.Helper()
	previous := judgeTestBudget
	judgeTestBudget = func(float64) time.Duration { return d }
	t.Cleanup(func() { judgeTestBudget = previous })
}

// stallingJudge is an in-process Judge0. With stall set it never answers in time (queued, hung):
// Submit blocks until the caller gives up, or a hard cap, then fails the way an HTTP client does.
// Otherwise it answers status immediately.
type stallingJudge struct {
	stall   time.Duration
	fail    error
	status  int
	calls   int
	lastReq dto.Judge0SubmissionDTO
}

func (j *stallingJudge) Submit(ctx context.Context, req dto.Judge0SubmissionDTO) (*dto.Judge0ResponseDTO, error) {
	j.calls++
	j.lastReq = req
	if j.fail != nil {
		return nil, j.fail
	}
	if j.stall > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(j.stall):
			return nil, errors.New("judge stalled past the hard cap")
		}
	}
	return &dto.Judge0ResponseDTO{Status: dto.Judge0StatusDTO{ID: j.status}, Stdout: req.ExpectedOutput, Time: "0.01", Memory: 16}, nil
}

func (j *stallingJudge) GetResult(context.Context, string) (*dto.Judge0ResponseDTO, error) {
	return nil, errors.New("not used")
}

func fourTests() []dto.ContestCodeTest {
	return []dto.ContestCodeTest{{Input: "1", ExpectedOutput: "1"}, {Input: "2", ExpectedOutput: "2"}, {Input: "3", ExpectedOutput: "3", Hidden: true}, {Input: "4", ExpectedOutput: "4", Hidden: true}}
}

func TestJudge0Wire_SendsWallTimeLimitCappedAtJudgeMax(t *testing.T) {
	cases := []struct {
		cpu      int
		wantWall float64
	}{{1, 2}, {2, 4}, {5, 10}, {10, 20}, {20, 20}, {0, 10}}
	for _, c := range cases {
		var body map[string]interface{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = w.Write([]byte(`{"status":{"id":3}}`))
		}))
		_, err := NewHTTPJudge0Client(server.URL).Submit(context.Background(), createJudge0Request("print(1)", 71, "", "", c.cpu, 131072))
		server.Close()
		if err != nil {
			t.Fatalf("cpu=%d: %v", c.cpu, err)
		}
		wall, ok := body["wall_time_limit"].(float64)
		if !ok || wall != c.wantWall {
			t.Errorf("cpu=%d: wall_time_limit=%v (present=%v), want %v", c.cpu, body["wall_time_limit"], ok, c.wantWall)
		}
		if cpuSent, _ := body["cpu_time_limit"].(float64); wall < cpuSent {
			t.Errorf("cpu=%d: wall %v below cpu %v", c.cpu, wall, cpuSent)
		}
	}
}

func TestJudgeCodeTests_Judge0TimeLimitIsAVerdict(t *testing.T) {
	judge := &stallingJudge{status: 5}
	req := dto.ContestCodeRequest{RequestID: uuid.New(), Language: "python", Code: "while True: pass"}
	result, err := judgeCodeTests(context.Background(), judge, req, uuid.New(), fourTests(), 100, 2, 128, true)
	if err != nil || result.Status != "TIME_LIMIT" || result.Passed != 0 || result.Score != 0 || len(result.Tests) != 4 || judge.calls != 4 {
		t.Fatalf("result=%+v err=%v calls=%d, want TIME_LIMIT on all 4 tests", result, err, judge.calls)
	}
}

func TestJudgeCodeTests_StalledJudgeIsUnavailableNotTimeLimit(t *testing.T) {
	useShortJudgeTestBudget(t, 100*time.Millisecond)
	judge := &stallingJudge{stall: 2 * time.Second}
	req := dto.ContestCodeRequest{RequestID: uuid.New(), Language: "python", Code: "print(1)"}
	started := time.Now()
	result, err := judgeCodeTests(context.Background(), judge, req, uuid.New(), fourTests(), 100, 2, 128, true)
	if !errors.Is(err, ErrJudgeUnavailable) || result != nil {
		t.Fatalf("judge that never answered graded as %+v err=%v, want ErrJudgeUnavailable", result, err)
	}
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("grading budget not applied: took %v", elapsed)
	}
}

func TestJudgeCodeTests_TransportFailureAndJudgeInternalErrorStayUnavailable(t *testing.T) {
	useShortJudgeTestBudget(t, time.Second)
	req := dto.ContestCodeRequest{RequestID: uuid.New(), Language: "python", Code: "print(1)"}
	for name, judge := range map[string]*stallingJudge{
		"connection refused": {fail: errors.New(`Post "http://localhost:2358/submissions": dial tcp: connection refused`)},
		"judge0 5xx":         {fail: errors.New("judge0 status 503")},
		"internal error":     {status: 13},
		"still queued":       {status: 1},
	} {
		if _, err := judgeCodeTests(context.Background(), judge, req, uuid.New(), fourTests(), 100, 2, 128, true); !errors.Is(err, ErrJudgeUnavailable) {
			t.Errorf("%s: err=%v, want ErrJudgeUnavailable", name, err)
		}
	}
}

func TestJudgeCodeTests_CallerCancellationIsNotATimeLimit(t *testing.T) {
	useShortJudgeTestBudget(t, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req := dto.ContestCodeRequest{RequestID: uuid.New(), Language: "python", Code: "print(1)"}
	if _, err := judgeCodeTests(ctx, &stallingJudge{stall: 2 * time.Second}, req, uuid.New(), fourTests(), 100, 2, 128, true); !errors.Is(err, ErrJudgeUnavailable) {
		t.Fatalf("caller deadline: err=%v, want ErrJudgeUnavailable (nothing graded)", err)
	}
}

func TestJudgeBudget_ScalesWithTestCountAndIsCappedForRequests(t *testing.T) {
	for _, cpu := range []int{1, 2, 5, 10, 20} {
		perTest := judgeTestBudget(judgeCPUSeconds(cpu))
		wall := time.Duration(judgeWallSeconds(judgeCPUSeconds(cpu)) * float64(time.Second))
		if perTest <= wall {
			t.Errorf("cpu=%d: per-test budget %v leaves no room past the %v wall limit", cpu, perTest, wall)
		}
		if perTest >= judgeHTTPBackstop {
			t.Errorf("cpu=%d: one job (%v) must end before the HTTP backstop %v", cpu, perTest, judgeHTTPBackstop)
		}
		for _, n := range []int{1, 4, 20} {
			total := judgeBudget(n, cpu)
			if total < time.Duration(n)*perTest {
				t.Errorf("cpu=%d tests=%d: overall %v < %d x %v", cpu, n, total, n, perTest)
			}
			request := judgeRequestBudget(n, cpu)
			if request > 90*time.Second || (total <= 90*time.Second && request != total) {
				t.Errorf("cpu=%d tests=%d: request budget %v, uncapped %v, want min(uncapped, 90 s)", cpu, n, request, total)
			}
		}
	}
	// The QA reproduction: 4 tests at the Judge0 maximum wall time no longer fit in a fixed 60 s.
	if judgeBudget(4, 20) <= 60*time.Second || judgeRequestBudget(4, 20) != 90*time.Second {
		t.Fatalf("budget does not scale: %v / request %v", judgeBudget(4, 20), judgeRequestBudget(4, 20))
	}
}
