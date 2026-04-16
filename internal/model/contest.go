package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ============================================================================
// CONTEST ENUMS
// ============================================================================

type ContestType string

const (
	ContestTypeCoding ContestType = "CODING"
	ContestTypeQuiz   ContestType = "QUIZ"
	ContestTypeMixed  ContestType = "MIXED"
)

type ContestStatus string

const (
	ContestStatusDraft     ContestStatus = "DRAFT"
	ContestStatusUpcoming  ContestStatus = "UPCOMING"
	ContestStatusActive    ContestStatus = "ACTIVE"
	ContestStatusEnded     ContestStatus = "ENDED"
	ContestStatusCancelled ContestStatus = "CANCELLED"
)

type ContestProblemType string

const (
	ContestProblemTypeCode           ContestProblemType = "CODE"
	ContestProblemTypeMultipleChoice ContestProblemType = "MULTIPLE_CHOICE"
	ContestProblemTypeShortAnswer    ContestProblemType = "SHORT_ANSWER"
)

type ContestProblemDifficulty string

const (
	ContestProblemDifficultyEasy   ContestProblemDifficulty = "EASY"
	ContestProblemDifficultyMedium ContestProblemDifficulty = "MEDIUM"
	ContestProblemDifficultyHard   ContestProblemDifficulty = "HARD"
)

type ContestSubmissionStatus string

const (
	ContestSubmissionStatusPending          ContestSubmissionStatus = "PENDING"
	ContestSubmissionStatusJudging          ContestSubmissionStatus = "JUDGING"
	ContestSubmissionStatusAccepted         ContestSubmissionStatus = "ACCEPTED"
	ContestSubmissionStatusWrongAnswer      ContestSubmissionStatus = "WRONG_ANSWER"
	ContestSubmissionStatusTimeLimit        ContestSubmissionStatus = "TIME_LIMIT"
	ContestSubmissionStatusMemoryLimit      ContestSubmissionStatus = "MEMORY_LIMIT"
	ContestSubmissionStatusRuntimeError     ContestSubmissionStatus = "RUNTIME_ERROR"
	ContestSubmissionStatusCompilationError ContestSubmissionStatus = "COMPILATION_ERROR"
)

// ============================================================================
// CONTEST
// ============================================================================

// Contest represents a competition/contest in the LMS
type Contest struct {
	BaseModel
	Title            string        `gorm:"type:varchar(255);not null" json:"title"`
	Slug             string        `gorm:"type:varchar(255);uniqueIndex;not null" json:"slug"`
	Description      *string       `gorm:"type:text" json:"description,omitempty"`
	BannerURL        *string       `gorm:"type:varchar(500);column:banner_url" json:"banner_url,omitempty"`
	Type             ContestType   `gorm:"type:varchar(20);not null;check:type IN ('CODING','QUIZ','MIXED')" json:"type"`
	Status           ContestStatus `gorm:"type:varchar(20);default:'DRAFT';check:status IN ('DRAFT','UPCOMING','ACTIVE','ENDED','CANCELLED')" json:"status"`
	StartTime        time.Time     `gorm:"not null" json:"start_time"`
	EndTime          time.Time     `gorm:"not null" json:"end_time"`
	Duration         *int          `gorm:"column:duration_minutes" json:"duration_minutes,omitempty"`
	MaxParticipants  int           `gorm:"default:0" json:"max_participants"`
	ParticipantCount int           `gorm:"default:0" json:"participant_count"`
	IsPublic         bool          `gorm:"default:true" json:"is_public"`
	CreatedBy        uuid.UUID     `gorm:"type:uuid;not null;index" json:"created_by"`
	OrganizationID   *uuid.UUID    `gorm:"type:uuid;index" json:"organization_id,omitempty"`

	// Relationships
	Creator      User                 `gorm:"foreignKey:CreatedBy" json:"-"`
	Organization *Organization        `gorm:"foreignKey:OrganizationID" json:"-"`
	Problems     []ContestProblem     `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	Participants []ContestParticipant `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Contest) TableName() string {
	return "contests"
}

// ============================================================================
// CONTEST PROBLEM
// ============================================================================

// ContestProblem represents a problem/question in a contest
type ContestProblem struct {
	ID           uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
	ContestID    uuid.UUID              `gorm:"type:uuid;not null;index" json:"contest_id"`
	Title        string                 `gorm:"type:varchar(255);not null" json:"title"`
	Description  string                 `gorm:"type:text;not null" json:"description"`
	Type         ContestProblemType     `gorm:"type:varchar(20);not null;check:type IN ('CODE','MULTIPLE_CHOICE','SHORT_ANSWER')" json:"type"`
	Difficulty   ContestProblemDifficulty `gorm:"type:varchar(20);not null;check:difficulty IN ('EASY','MEDIUM','HARD')" json:"difficulty"`
	Points       int                    `gorm:"not null" json:"points"`
	DisplayOrder int                    `gorm:"default:0" json:"display_order"`
	TimeLimit    *int                   `gorm:"column:time_limit_seconds" json:"time_limit_seconds,omitempty"`
	MemoryLimit  *int                   `gorm:"column:memory_limit_mb" json:"memory_limit_mb,omitempty"`
	InputFormat  *string                `gorm:"type:text" json:"input_format,omitempty"`
	OutputFormat *string                `gorm:"type:text" json:"output_format,omitempty"`
	SampleInput  *string                `gorm:"type:text" json:"sample_input,omitempty"`
	SampleOutput *string                `gorm:"type:text" json:"sample_output,omitempty"`
	TestCases    datatypes.JSON         `gorm:"type:jsonb;default:'[]'" json:"test_cases"`
	Options      datatypes.JSON         `gorm:"type:jsonb;default:'[]'" json:"options"`
	CorrectAnswer *string               `gorm:"type:text" json:"correct_answer,omitempty"`

	// Relationships
	Contest Contest `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ContestProblem) TableName() string {
	return "contest_problems"
}

// ============================================================================
// CONTEST PARTICIPANT
// ============================================================================

// ContestParticipant represents a user participating in a contest
type ContestParticipant struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`
	ContestID  uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_contest_participant_unique,priority:1" json:"contest_id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_contest_participant_unique,priority:2" json:"user_id"`
	TotalScore int        `gorm:"default:0" json:"total_score"`
	Rank       *int       `json:"rank,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Relationships
	Contest Contest `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	User    User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ContestParticipant) TableName() string {
	return "contest_participants"
}

// ============================================================================
// CONTEST SUBMISSION
// ============================================================================

// ContestSubmission represents an individual problem submission in a contest
type ContestSubmission struct {
	ID            uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt     time.Time              `gorm:"autoCreateTime" json:"created_at"`
	ContestID     uuid.UUID              `gorm:"type:uuid;not null;index" json:"contest_id"`
	ProblemID     uuid.UUID              `gorm:"type:uuid;not null;index" json:"problem_id"`
	UserID        uuid.UUID              `gorm:"type:uuid;not null;index" json:"user_id"`
	Code          *string                `gorm:"type:text" json:"code,omitempty"`
	Language      *string                `gorm:"type:varchar(20)" json:"language,omitempty"`
	Answer        *string                `gorm:"type:text" json:"answer,omitempty"`
	Score         int                    `gorm:"default:0" json:"score"`
	MaxScore      int                    `gorm:"not null" json:"max_score"`
	Status        ContestSubmissionStatus `gorm:"type:varchar(20);default:'PENDING';check:status IN ('PENDING','JUDGING','ACCEPTED','WRONG_ANSWER','TIME_LIMIT','MEMORY_LIMIT','RUNTIME_ERROR','COMPILATION_ERROR')" json:"status"`
	ExecutionTime *int                   `gorm:"column:execution_time_ms" json:"execution_time_ms,omitempty"`
	MemoryUsed    *int                   `gorm:"column:memory_used_kb" json:"memory_used_kb,omitempty"`
	Output        *string                `gorm:"type:text" json:"output,omitempty"`

	// Relationships
	Contest Contest        `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	Problem ContestProblem `gorm:"foreignKey:ProblemID;constraint:OnDelete:CASCADE" json:"-"`
	User    User           `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ContestSubmission) TableName() string {
	return "contest_submissions"
}
