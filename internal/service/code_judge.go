package service

import (
	"context"
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
	// Backstop for the HTTP client; must outlast one job at the maximum wall limit plus overhead.
	judgeHTTPBackstop = 60 * time.Second
	// Lesson and contest grading answer within the request; the web proxy for them gives up at
	// 100 s, so the whole run is capped below that.
	judgeRequestBudgetCap = 90 * time.Second
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

// judgeTestBudget is the time one Judge0 job may take: its wall limit plus overhead. A slow program
// is stopped by Judge0 itself (status 5); our deadline firing means the judge is queued, overloaded
// or hung. A variable so tests can shrink it.
var judgeTestBudget = func(cpuSeconds float64) time.Duration {
	return time.Duration(judgeWallSeconds(cpuSeconds)*float64(time.Second)) + judgeTestOverhead
}

// judgeBudget is the deadline for grading n tests. It grows with the test count, so a submission
// whose tests all run to the wall limit is still graded instead of reported as an outage.
func judgeBudget(tests, timeLimit int) time.Duration {
	if tests < 1 {
		tests = 1
	}
	// One spare job's worth of margin for the queue and the database work around grading.
	return time.Duration(tests+1) * judgeTestBudget(judgeCPUSeconds(timeLimit))
}

// judgeRequestBudget is judgeBudget capped for grading done inside an HTTP request. Tests the
// judge has not answered by then make the run unavailable, never a verdict.
func judgeRequestBudget(tests, timeLimit int) time.Duration {
	if budget := judgeBudget(tests, timeLimit); budget < judgeRequestBudgetCap {
		return budget
	}
	return judgeRequestBudgetCap
}

// judgeOneTest submits one job. Only an answer from Judge0 is a verdict: a transport error, a 5xx,
// an expired deadline or a status outside 3..12 (queued, internal error) is ErrJudgeUnavailable.
func judgeOneTest(ctx context.Context, judge Judge0Client, req dto.Judge0SubmissionDTO) (*dto.Judge0ResponseDTO, error) {
	judged, err := judge.Submit(ctx, req)
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
	ctx, cancel := context.WithTimeout(ctx, judgeRequestBudget(len(tests), timeLimit))
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
