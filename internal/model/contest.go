package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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

// Trạng thái cuộc thi: xem contest_status.go (SSOT). Hằng cũ UPCOMING/ACTIVE/ENDED đã bỏ —
// thời gian quyết định phase lúc đọc, không còn job đổi status theo giờ.

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

// Contest — cuộc thi trắc nghiệm gắn đúng 1 quiz standalone (contract §1.2).
type Contest struct {
	BaseModel
	Title            string      `gorm:"type:varchar(255);not null" json:"title"`
	Slug             string      `gorm:"type:varchar(255);uniqueIndex:idx_contests_slug;not null" json:"slug"`
	Description      *string     `gorm:"type:text" json:"description,omitempty"`
	BannerURL        *string     `gorm:"type:varchar(500);column:banner_url" json:"banner_url,omitempty"`
	Type             ContestType `gorm:"type:varchar(20);not null;check:type IN ('CODING','QUIZ','MIXED')" json:"type"`
	Status           string      `gorm:"type:varchar(20);not null;default:'DRAFT';check:status IN ('DRAFT','PENDING_REVIEW','PUBLISHED','REJECTED','CANCELLED')" json:"status"`
	StartTime        time.Time   `gorm:"not null" json:"start_time"`
	EndTime          time.Time   `gorm:"not null" json:"end_time"`
	DurationMinutes  *int        `gorm:"column:duration_minutes" json:"duration_minutes,omitempty"`
	MaxParticipants  int         `gorm:"default:0" json:"max_participants"`
	ParticipantCount int         `gorm:"default:0" json:"participant_count"` // bộ đếm atomic chặn vượt chỗ
	IsPublic         bool        `gorm:"default:true" json:"is_public"`
	CreatedBy        uuid.UUID   `gorm:"type:uuid;not null;index" json:"created_by"`
	OrganizationID   *uuid.UUID  `gorm:"type:uuid;index" json:"organization_id,omitempty"`

	QuizID                   *uuid.UUID       `gorm:"type:uuid;uniqueIndex:idx_contests_quiz_id" json:"quiz_id,omitempty"`
	CourseID                 *uuid.UUID       `gorm:"type:uuid;index" json:"course_id,omitempty"`
	CertificateMinPercentage *decimal.Decimal `gorm:"type:decimal(5,2)" json:"certificate_min_percentage,omitempty"`
	SubmittedAt              *time.Time       `json:"submitted_at,omitempty"`
	ReviewedBy               *uuid.UUID       `gorm:"type:uuid" json:"reviewed_by,omitempty"`
	ReviewedAt               *time.Time       `json:"reviewed_at,omitempty"`
	RejectReason             *string          `gorm:"type:text" json:"reject_reason,omitempty"`
	CancelReason             *string          `gorm:"type:text" json:"cancel_reason,omitempty"`
	FinalizedAt              *time.Time       `json:"finalized_at,omitempty"`
	FinalizedBy              *uuid.UUID       `gorm:"type:uuid" json:"finalized_by,omitempty"`

	// Relationships
	Creator      User                 `gorm:"foreignKey:CreatedBy" json:"-"`
	Organization *Organization        `gorm:"foreignKey:OrganizationID" json:"-"`
	Quiz         *Quiz                `gorm:"foreignKey:QuizID;constraint:OnDelete:RESTRICT" json:"-"`
	Course       *Course              `gorm:"foreignKey:CourseID" json:"-"`
	Prizes       []ContestPrize       `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	Problems     []ContestProblem     `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	Participants []ContestParticipant `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Contest) TableName() string {
	return "contests"
}

// ============================================================================
// CONTEST PROBLEM (giữ bảng cho loại CODING sau này; MVP không có route nào dùng)
// ============================================================================

type ContestProblem struct {
	ID            uuid.UUID                `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt     time.Time                `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time                `gorm:"autoUpdateTime" json:"updated_at"`
	ContestID     uuid.UUID                `gorm:"type:uuid;not null;index" json:"contest_id"`
	Title         string                   `gorm:"type:varchar(255);not null" json:"title"`
	Description   string                   `gorm:"type:text;not null" json:"description"`
	Type          ContestProblemType       `gorm:"type:varchar(20);not null;check:type IN ('CODE','MULTIPLE_CHOICE','SHORT_ANSWER')" json:"type"`
	Difficulty    ContestProblemDifficulty `gorm:"type:varchar(20);not null;check:difficulty IN ('EASY','MEDIUM','HARD')" json:"difficulty"`
	Points        int                      `gorm:"not null" json:"points"`
	DisplayOrder  int                      `gorm:"default:0" json:"display_order"`
	TimeLimit     *int                     `gorm:"column:time_limit_seconds" json:"time_limit_seconds,omitempty"`
	MemoryLimit   *int                     `gorm:"column:memory_limit_mb" json:"memory_limit_mb,omitempty"`
	InputFormat   *string                  `gorm:"type:text" json:"input_format,omitempty"`
	OutputFormat  *string                  `gorm:"type:text" json:"output_format,omitempty"`
	SampleInput   *string                  `gorm:"type:text" json:"sample_input,omitempty"`
	SampleOutput  *string                  `gorm:"type:text" json:"sample_output,omitempty"`
	TestCases     datatypes.JSON           `gorm:"type:jsonb;default:'[]'" json:"test_cases"`
	Options       datatypes.JSON           `gorm:"type:jsonb;default:'[]'" json:"options"`
	CorrectAnswer *string                  `gorm:"type:text" json:"correct_answer,omitempty"`

	Contest Contest `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ContestProblem) TableName() string {
	return "contest_problems"
}

// ============================================================================
// CONTEST PARTICIPANT
// ============================================================================

// ContestParticipant — một thí sinh đã đăng ký. Điểm/thời gian/giờ nộp LUÔN đọc từ quiz_attempts
// qua AttemptID (attempt bất biến sau khi nộp); TotalScore/StartedAt/FinishedAt giữ cho CODING,
// MVP không đọc/ghi. Rank chỉ ghi khi chốt kết quả.
type ContestParticipant struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`
	ContestID  uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_contest_participant_unique,priority:1" json:"contest_id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_contest_participant_unique,priority:2" json:"user_id"`
	AttemptID  *uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_contest_participants_attempt_id" json:"attempt_id,omitempty"`
	TotalScore int        `gorm:"default:0" json:"total_score"`
	Rank       *int       `json:"rank,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	Contest Contest      `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	User    User         `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Attempt *QuizAttempt `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT" json:"-"`
}

func (ContestParticipant) TableName() string {
	return "contest_participants"
}

// ============================================================================
// CONTEST SUBMISSION (giữ bảng cho CODING; MVP không dùng)
// ============================================================================

type ContestSubmission struct {
	ID            uuid.UUID               `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt     time.Time               `gorm:"autoCreateTime" json:"created_at"`
	ContestID     uuid.UUID               `gorm:"type:uuid;not null;index" json:"contest_id"`
	ProblemID     uuid.UUID               `gorm:"type:uuid;not null;index" json:"problem_id"`
	UserID        uuid.UUID               `gorm:"type:uuid;not null;index" json:"user_id"`
	Code          *string                 `gorm:"type:text" json:"code,omitempty"`
	Language      *string                 `gorm:"type:varchar(20)" json:"language,omitempty"`
	Answer        *string                 `gorm:"type:text" json:"answer,omitempty"`
	Score         int                     `gorm:"default:0" json:"score"`
	MaxScore      int                     `gorm:"not null" json:"max_score"`
	Status        ContestSubmissionStatus `gorm:"type:varchar(20);default:'PENDING';check:status IN ('PENDING','JUDGING','ACCEPTED','WRONG_ANSWER','TIME_LIMIT','MEMORY_LIMIT','RUNTIME_ERROR','COMPILATION_ERROR')" json:"status"`
	ExecutionTime *int                    `gorm:"column:execution_time_ms" json:"execution_time_ms,omitempty"`
	MemoryUsed    *int                    `gorm:"column:memory_used_kb" json:"memory_used_kb,omitempty"`
	Output        *string                 `gorm:"type:text" json:"output,omitempty"`

	Contest Contest        `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	Problem ContestProblem `gorm:"foreignKey:ProblemID;constraint:OnDelete:CASCADE" json:"-"`
	User    User           `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ContestSubmission) TableName() string {
	return "contest_submissions"
}

// ============================================================================
// CONTEST PRIZE / AWARD (contract §1.4, §1.5)
// ============================================================================

// ContestPrize — một khoảng hạng [RankFrom, RankTo] nhận chứng nhận và/hoặc voucher.
type ContestPrize struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt        time.Time  `gorm:"autoCreateTime" json:"created_at"`
	ContestID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"contest_id"`
	RankFrom         int        `gorm:"not null;check:chk_contest_prizes_rank,rank_from >= 1 AND rank_to >= rank_from" json:"rank_from"`
	RankTo           int        `gorm:"not null" json:"rank_to"`
	GrantCertificate bool       `gorm:"not null;default:true" json:"grant_certificate"`
	VoucherID        *uuid.UUID `gorm:"type:uuid" json:"voucher_id,omitempty"`

	Voucher *Voucher `gorm:"foreignKey:VoucherID" json:"-"`
}

func (ContestPrize) TableName() string {
	return "contest_prizes"
}

// ContestAward — thứ đã phát cho một thí sinh khi chốt. Rank NULL = chỉ đạt ngưỡng chứng nhận.
type ContestAward struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ContestID         uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_contest_awards_contest_user,priority:1" json:"contest_id"`
	UserID            uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_contest_awards_contest_user,priority:2" json:"user_id"`
	Rank              *int       `json:"rank,omitempty"`
	CertificateNumber *string    `gorm:"type:varchar(50);uniqueIndex:idx_contest_awards_certificate_number" json:"certificate_number,omitempty"`
	UserVoucherID     *uuid.UUID `gorm:"type:uuid" json:"user_voucher_id,omitempty"`
	IssuedAt          time.Time  `gorm:"not null" json:"issued_at"`

	Contest     Contest      `gorm:"foreignKey:ContestID" json:"-"`
	User        User         `gorm:"foreignKey:UserID" json:"-"`
	UserVoucher *UserVoucher `gorm:"foreignKey:UserVoucherID" json:"-"`
}

func (ContestAward) TableName() string {
	return "contest_awards"
}
