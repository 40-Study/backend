package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type AssignmentDifficulty string

const (
	DifficultyEasy   AssignmentDifficulty = "easy"
	DifficultyMedium AssignmentDifficulty = "medium"
	DifficultyHard   AssignmentDifficulty = "hard"
)

type Assignment struct {
	BaseModel
	SessionID   uuid.UUID            `gorm:"type:uuid;not null;index" json:"session_id"`
	Title       string               `gorm:"type:varchar(255);not null" json:"title"`
	Description string               `gorm:"type:text;not null" json:"description"`
	Difficulty  AssignmentDifficulty `gorm:"type:varchar(20);default:'medium'" json:"difficulty"`
	Language    pq.StringArray       `gorm:"type:text[];not null" json:"language"`
	StarterCode string               `gorm:"type:text" json:"starter_code"`
	TimeLimit   int                  `gorm:"default:2" json:"time_limit"`
	MemoryLimit int                  `gorm:"default:256" json:"memory_limit"`
	IsPublished bool                 `gorm:"default:false;index" json:"is_published"`
	PublishedAt *time.Time           `gorm:"type:timestamp" json:"published_at,omitempty"`

	Session     *LivestreamSession `gorm:"foreignKey:SessionID" json:"-"`
	TestCases   []TestCase         `gorm:"foreignKey:AssignmentID;constraint:OnDelete:CASCADE" json:"test_cases,omitempty"`
	Submissions []Submission       `gorm:"foreignKey:AssignmentID;constraint:OnDelete:CASCADE" json:"submissions,omitempty"`
}

func (Assignment) TableName() string {
	return "assignments"
}

type TestCase struct {
	BaseModel
	AssignmentID   uuid.UUID `gorm:"type:uuid;not null;index" json:"assignment_id"`
	Input          string    `gorm:"type:text;not null" json:"input"`
	ExpectedOutput string    `gorm:"type:text;not null" json:"expected_output"`
	IsHidden       bool      `gorm:"default:false" json:"is_hidden"`
	DisplayOrder   int       `gorm:"default:0" json:"display_order"`

	Assignment *Assignment `gorm:"foreignKey:AssignmentID" json:"-"`
}

func (TestCase) TableName() string {
	return "test_cases"
}

type SubmissionVerdict string

const (
	VerdictPending           SubmissionVerdict = "pending"
	VerdictAccepted          SubmissionVerdict = "accepted"
	VerdictWrongAnswer       SubmissionVerdict = "wrong_answer"
	VerdictCompilationError  SubmissionVerdict = "compilation_error"
	VerdictRuntimeError      SubmissionVerdict = "runtime_error"
	VerdictTimeLimitExceed   SubmissionVerdict = "time_limit_exceeded"
	VerdictMemoryLimitExceed SubmissionVerdict = "memory_limit_exceeded"
)

func (v SubmissionVerdict) String() string {
	return string(v)
}

type Submission struct {
	BaseModel
	AssignmentID    uuid.UUID         `gorm:"type:uuid;not null;index" json:"assignment_id"`
	UserID          uuid.UUID         `gorm:"type:uuid;not null;index" json:"user_id"`
	Language        string            `gorm:"type:varchar(50);not null" json:"language"`
	Code            string            `gorm:"type:text;not null" json:"code"`
	Verdict         SubmissionVerdict `gorm:"type:varchar(30);default:'pending'" json:"verdict"`
	ExecutionTime   int               `gorm:"default:0" json:"execution_time"`
	MemoryUsed      int               `gorm:"default:0" json:"memory_used"`
	TestCasesPassed int               `gorm:"default:0" json:"test_cases_passed"`
	TotalTestCases  int               `gorm:"default:0" json:"total_test_cases"`

	Assignment *Assignment `gorm:"foreignKey:AssignmentID" json:"-"`
	User       *User       `gorm:"foreignKey:UserID" json:"-"`
}

func (Submission) TableName() string {
	return "submissions"
}
