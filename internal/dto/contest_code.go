package dto

import (
	"github.com/google/uuid"
	"time"
)

type ContestCodeTest struct {
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	Hidden         bool   `json:"hidden"`
}
type ContestCodeProblemInput struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Difficulty  string            `json:"difficulty"`
	Points      int               `json:"points"`
	TimeLimit   int               `json:"time_limit_seconds"`
	MemoryLimit int               `json:"memory_limit_mb"`
	Tests       []ContestCodeTest `json:"tests"`
}
type ContestCodeProblemDTO struct {
	ID          uuid.UUID         `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Difficulty  string            `json:"difficulty"`
	Points      int               `json:"points"`
	TimeLimit   int               `json:"time_limit_seconds"`
	MemoryLimit int               `json:"memory_limit_mb"`
	Samples     []ContestCodeTest `json:"samples"`
	Tests       []ContestCodeTest `json:"tests,omitempty"` // owner/admin only
}
type ContestCodeRequest struct {
	RequestID uuid.UUID `json:"request_id"`
	ProblemID uuid.UUID `json:"problem_id"`
	Language  string    `json:"language"`
	Code      string    `json:"code"`
	Stdin     string    `json:"stdin"`
}
type ContestCodeTestResult struct {
	Number   int    `json:"number"`
	Hidden   bool   `json:"hidden"`
	Status   string `json:"status"`
	Output   string `json:"output,omitempty"`
	Expected string `json:"expected,omitempty"`
	Error    string `json:"error,omitempty"`
}
type ContestCodeResult struct {
	ID            string                  `json:"id,omitempty"`
	ProblemID     uuid.UUID               `json:"problem_id"`
	Language      string                  `json:"language"`
	Status        string                  `json:"status"`
	Score         int                     `json:"score"`
	MaxScore      int                     `json:"max_score"`
	Passed        int                     `json:"passed"`
	Total         int                     `json:"total"`
	ExecutionTime int                     `json:"execution_time_ms"`
	MemoryUsed    int                     `json:"memory_used_kb"`
	Tests         []ContestCodeTestResult `json:"tests"`
	CreatedAt     time.Time               `json:"created_at"`
}
type ContestCodeSession struct {
	Problems    []ContestCodeProblemDTO `json:"problems"`
	Submissions []ContestCodeResult     `json:"submissions"`
	StartedAt   *time.Time              `json:"started_at"`
	DeadlineAt  *time.Time              `json:"deadline_at"`
	FinishedAt  *time.Time              `json:"finished_at"`
	ServerTime  time.Time               `json:"server_time"`
	Score       int                     `json:"score"`
	TotalPoints int                     `json:"total_points"`
	CanSubmit   bool                    `json:"can_submit"`
}
