package service

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"strconv"
	"strings"
	"study.com/v1/internal/dto"
	"time"
)

const (
	// Judge0 answers 422 to a wall_time_limit above MAX_WALL_TIME_LIMIT (judge/judge0.conf).
	judgeMaxWallSeconds = 20.0
	// Compile, queue and transport time a job may take on top of the program's wall limit.
	judgeTestOverhead = 8 * time.Second
	// Backstop for the HTTP client; must outlast the largest per-test budget (20 s + overhead),
	// otherwise a slow program ends as a transport error and reads as an outage.
	judgeHTTPBackstop    = 60 * time.Second
	judgeStatusTimeLimit = 5
)

// judgeCPUSeconds is the CPU limit sent to Judge0: the problem's seconds, else 5 s.
func judgeCPUSeconds(timeLimit int) float64 {
	if timeLimit > 0 && timeLimit <= 20 {
		return float64(timeLimit)
	}
	return 5
}

// judgeWallSeconds bounds sleeping or blocked programs, which the CPU limit never stops.
func judgeWallSeconds(cpuSeconds float64) float64 {
	if wall := 2 * cpuSeconds; wall < judgeMaxWallSeconds {
		return wall
	}
	return judgeMaxWallSeconds
}

// judgeTestBudget is how long one Judge0 job may take before it counts as TIME_LIMIT.
// A variable so tests can shrink it.
var judgeTestBudget = func(cpuSeconds float64) time.Duration {
	return time.Duration(judgeWallSeconds(cpuSeconds)*float64(time.Second)) + judgeTestOverhead
}

// judgeBudget is the backstop for grading n tests: it grows with the test count, so a submission
// whose tests all hit the wall limit still gets graded instead of reported as an outage.
func judgeBudget(tests, timeLimit int) time.Duration {
	if tests < 1 {
		tests = 1
	}
	return time.Duration(tests)*judgeTestBudget(judgeCPUSeconds(timeLimit)) + judgeTestOverhead
}

// judgeOneTest runs one job inside its own budget. A job that outlives the budget is the program's
// fault (TIME_LIMIT). A transport error, a 5xx, the caller's own deadline or a status outside
// 3..12 (queued, internal error) means the judge is unavailable, never a verdict.
func judgeOneTest(ctx context.Context, judge Judge0Client, req dto.Judge0SubmissionDTO) (*dto.Judge0ResponseDTO, error) {
	testCtx, cancel := context.WithTimeout(ctx, judgeTestBudget(req.CPUTimeLimit))
	defer cancel()
	judged, err := judge.Submit(testCtx, req)
	if err != nil && ctx.Err() == nil && errors.Is(testCtx.Err(), context.DeadlineExceeded) {
		return &dto.Judge0ResponseDTO{Status: dto.Judge0StatusDTO{ID: judgeStatusTimeLimit, Description: "Time Limit Exceeded"}}, nil
	}
	if err != nil || judged == nil || judged.Status.ID < 3 || judged.Status.ID > 12 {
		return nil, ErrJudgeUnavailable
	}
	return judged, nil
}

// Shared grading for contest and lesson exercises. Hidden data never reaches the result.
func judgeCodeTests(ctx context.Context, judge Judge0Client, req dto.ContestCodeRequest, problemID uuid.UUID, tests []dto.ContestCodeTest, points, timeLimit, memoryLimit int, submit bool) (*dto.ContestCodeResult, error) {
	languageID, ok := CodeLanguageID(req.Language)
	if !ok || len(tests) == 0 {
		return nil, invalidField("language", "Ngôn ngữ hoặc test không hợp lệ.")
	}
	ctx, cancel := context.WithTimeout(ctx, judgeBudget(len(tests), timeLimit))
	defer cancel()
	result := &dto.ContestCodeResult{ProblemID: problemID, Language: req.Language, MaxScore: points, Total: len(tests), Status: "ACCEPTED", Tests: []dto.ContestCodeTestResult{}, CreatedAt: time.Now()}
	for i, t := range tests {
		judged, err := judgeOneTest(ctx, judge, createJudge0Request(req.Code, languageID, t.Input, t.ExpectedOutput, timeLimit, memoryLimit*1024))
		if err != nil {
			return nil, err
		}
		// Judge0 omits an empty expected_output. Still grade programs whose expected stdout is empty.
		if submit && t.ExpectedOutput == "" && judged.Status.ID == 3 && strings.TrimSpace(judged.Stdout) != "" {
			judged.Status.ID = 4
		}
		verdict := codeVerdict(judged.Status.ID)
		if verdict == "ACCEPTED" {
			result.Passed++
		} else if result.Status == "ACCEPTED" {
			result.Status = verdict
		}
		test := dto.ContestCodeTestResult{Number: i + 1, Hidden: t.Hidden, Status: verdict}
		if !t.Hidden {
			test.Output = judged.Stdout
			test.Expected = t.ExpectedOutput
			test.Error = judged.CompileOutput + judged.Stderr
		}
		result.Tests = append(result.Tests, test)
		seconds, _ := strconv.ParseFloat(judged.Time, 64)
		if ms := int(seconds * 1000); ms > result.ExecutionTime {
			result.ExecutionTime = ms
		}
		if judged.Memory > result.MemoryUsed {
			result.MemoryUsed = judged.Memory
		}
		// Compilation does not depend on stdin. Do not compile the same broken source again.
		if judged.Status.ID == 6 {
			for j := i + 1; j < len(tests); j++ {
				rest := dto.ContestCodeTestResult{Number: j + 1, Hidden: tests[j].Hidden, Status: "COMPILATION_ERROR"}
				if !rest.Hidden {
					rest.Error = judged.CompileOutput
				}
				result.Tests = append(result.Tests, rest)
			}
			break
		}
	}
	if submit {
		result.Score = points * result.Passed / result.Total
	}
	return result, nil
}
