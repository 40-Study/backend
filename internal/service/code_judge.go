package service

import (
	"context"
	"github.com/google/uuid"
	"strconv"
	"strings"
	"study.com/v1/internal/dto"
	"time"
)

// Shared grading for contest and lesson exercises. Hidden data never reaches the result.
func judgeCodeTests(ctx context.Context, judge Judge0Client, req dto.ContestCodeRequest, problemID uuid.UUID, tests []dto.ContestCodeTest, points, timeLimit, memoryLimit int, submit bool) (*dto.ContestCodeResult, error) {
	languageID, ok := CodeLanguageID(req.Language)
	if !ok || len(tests) == 0 {
		return nil, invalidField("language", "Ngôn ngữ hoặc test không hợp lệ.")
	}
	result := &dto.ContestCodeResult{ProblemID: problemID, Language: req.Language, MaxScore: points, Total: len(tests), Status: "ACCEPTED", Tests: []dto.ContestCodeTestResult{}, CreatedAt: time.Now()}
	for i, t := range tests {
		judged, err := judge.Submit(ctx, createJudge0Request(req.Code, languageID, t.Input, t.ExpectedOutput, timeLimit, memoryLimit*1024))
		if err != nil || judged == nil || judged.Status.ID < 3 || judged.Status.ID > 12 {
			return nil, ErrJudgeUnavailable
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
