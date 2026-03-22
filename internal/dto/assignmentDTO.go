package dto

import "github.com/google/uuid"

type CreateAssignmentDTO struct {
	SessionID   string   `json:"session_id" validate:"required,uuid"`
	Title       string   `json:"title" validate:"required,min=3,max=255"`
	Description string   `json:"description" validate:"required"`
	Difficulty  string   `json:"difficulty" validate:"required,oneof=easy medium hard"`
	Language    []string `json:"language" validate:"required"`
	StarterCode string   `json:"starter_code"`
	TimeLimit   int      `json:"time_limit"`
	MemoryLimit int      `json:"memory_limit"`
	StartTime   *string  `json:"start_time"`
	EndTime     *string  `json:"end_time"`
}

type UpdateAssignmentDTO struct {
	Title       *string   `json:"title" validate:"omitempty,min=3,max=255"`
	Description *string   `json:"description"`
	Difficulty  *string   `json:"difficulty" validate:"omitempty,oneof=easy medium hard"`
	Language    *[]string `json:"language"`
	StarterCode *string   `json:"starter_code"`
	TimeLimit   *int      `json:"time_limit"`
	MemoryLimit *int      `json:"memory_limit"`
	StartTime   *string   `json:"start_time"`
	EndTime     *string   `json:"end_time"`
}

type AssignmentResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	SessionID   uuid.UUID `json:"session_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Difficulty  string    `json:"difficulty"`
	Language    []string  `json:"language"`
	StarterCode string    `json:"starter_code"`
	TimeLimit   int       `json:"time_limit"`
	MemoryLimit int       `json:"memory_limit"`
	IsPublished bool      `json:"is_published"`
	PublishedAt *string   `json:"published_at,omitempty"`
	StartTime   *string   `json:"start_time,omitempty"`
	EndTime     *string   `json:"end_time,omitempty"`
	CreatedAt   string    `json:"created_at"`
}

type AssignmentListDTO struct {
	Data     []AssignmentResponseDTO `json:"data"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type CreateTestCaseDTO struct {
	Input          string `json:"input" validate:"required"`
	ExpectedOutput string `json:"expected_output" validate:"required"`
	IsHidden       bool   `json:"is_hidden"`
	DisplayOrder   int    `json:"display_order"`
}

type TestCaseResponseDTO struct {
	ID             uuid.UUID `json:"id"`
	AssignmentID   uuid.UUID `json:"assignment_id"`
	Input          string    `json:"input"`
	ExpectedOutput string    `json:"expected_output,omitempty"`
	IsHidden       bool      `json:"is_hidden"`
	DisplayOrder   int       `json:"display_order"`
}

type PublishAssignmentDTO struct {
	SessionID string `json:"session_id" validate:"required,uuid"`
}

type ImportTestCasesDTO struct {
	TestCases []CreateTestCaseDTO `json:"test_cases" validate:"required,min=1,dive"`
}

type SandboxResponseDTO struct {
	Assignment     AssignmentResponseDTO  `json:"assignment"`
	SampleTests    []TestCaseResponseDTO  `json:"sample_tests"`
	LastSubmission *SubmissionSnapshotDTO `json:"last_submission,omitempty"`
}

type SubmissionSnapshotDTO struct {
	ID              uuid.UUID `json:"id"`
	Language        string    `json:"language"`
	Code            string    `json:"code"`
	Verdict         string    `json:"verdict"`
	TestCasesPassed int       `json:"test_cases_passed"`
	TotalTestCases  int       `json:"total_test_cases"`
	SubmittedAt     string    `json:"submitted_at"`
}
