package dto

import "github.com/google/uuid"

type CreateSubmissionDTO struct {
	AssignmentID string `json:"assignment_id" validate:"required,uuid"`
	UserID       string `json:"user_id" validate:"required,uuid"`
	Language     string `json:"language" validate:"required"`
	Code         string `json:"code" validate:"required"`
}

type SubmissionResponseDTO struct {
	ID              uuid.UUID `json:"id"`
	AssignmentID    uuid.UUID `json:"assignment_id"`
	UserID          uuid.UUID `json:"user_id"`
	Code            string    `json:"code"`
	Verdict         string    `json:"verdict"`
	ExecutionTime   int       `json:"execution_time"`
	MemoryUsed      int       `json:"memory_used"`
	TestCasesPassed int       `json:"test_cases_passed"`
	TotalTestCases  int       `json:"total_test_cases"`
	CreatedAt       string    `json:"created_at"`
}

type SubmissionListDTO struct {
	Data     []SubmissionResponseDTO `json:"data"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type Judge0SubmissionDTO struct {
	SourceCode     string `json:"source_code"`
	LanguageID     int    `json:"language_id"`
	Stdin          string `json:"stdin"`
	ExpectedOutput string `json:"expected_output"`
	CPUTimeLimit   int    `json:"cpu_time_limit"`
	MemoryLimit    int    `json:"memory_limit"`
}

type Judge0ResponseDTO struct {
	Token         string          `json:"token"`
	Stdout        string          `json:"stdout"`
	Stderr        string          `json:"stderr"`
	CompileOutput string          `json:"compile_output"`
	Message       string          `json:"message"`
	Status        Judge0StatusDTO `json:"status"`
	Time          string          `json:"time"`
	Memory        string          `json:"memory"`
}

type Judge0StatusDTO struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

type RunCodeDTO struct {
	AssignmentID string `json:"assignment_id" validate:"required,uuid"`
	Language     string `json:"language" validate:"required"`
	Code         string `json:"code" validate:"required"`
}

type RunCodeResponseDTO struct {
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	Verdict  string `json:"verdict"`
	ExecTime int    `json:"exec_time"`
	MemUsed  int    `json:"mem_used"`
}

type RunCustomCodeDTO struct {
	AssignmentID string `json:"assignment_id" validate:"required,uuid"`
	Language     string `json:"language" validate:"required"`
	Code         string `json:"code" validate:"required"`
	CustomInput  string `json:"custom_input" validate:"required"`
}
