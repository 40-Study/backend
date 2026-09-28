package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MVP "Cuộc thi" — shape đúng contract §2.1 (plans/260927-2055-role-based-ux-qa/contest-feature/
// contract.md). KHÔNG DTO nào ở file này chứa is_correct/correct_answer_ids/explanation/answer_key;
// đáp án chỉ có ở ContestMyResultDTO.Questions (QuizAttemptAnswerDTO) khi phase ENDED/FINALIZED.

// ===== Request =====

type ContestPrizeInput struct {
	RankFrom         int        `json:"rank_from" validate:"min=1"`
	RankTo           int        `json:"rank_to" validate:"min=1,max=100"`
	GrantCertificate bool       `json:"grant_certificate"`
	VoucherID        *uuid.UUID `json:"voucher_id"`
}

type ContestUpsertRequest struct {
	Title                    string              `json:"title" validate:"required,min=2,max=255"`
	Description              *string             `json:"description"`
	BannerURL                *string             `json:"banner_url" validate:"omitempty,url,safe_url,max=500"`
	QuizID                   uuid.UUID           `json:"quiz_id" validate:"required"`
	CourseID                 *uuid.UUID          `json:"course_id"`
	StartTime                time.Time           `json:"start_time" validate:"required"`
	EndTime                  time.Time           `json:"end_time" validate:"required"`
	DurationMinutes          int                 `json:"duration_minutes" validate:"min=1,max=600"`
	MaxParticipants          int                 `json:"max_participants" validate:"min=0,max=100000"`
	IsPublic                 bool                `json:"is_public"`
	CertificateMinPercentage *decimal.Decimal    `json:"certificate_min_percentage"`
	Prizes                   []ContestPrizeInput `json:"prizes" validate:"max=10,dive"`
}

type ContestReasonRequest struct {
	Reason string `json:"reason"`
}

type ContestPrizesRequest struct {
	Prizes []ContestPrizeInput `json:"prizes" validate:"max=10,dive"`
}

type ContestSubmitRequest struct {
	AttemptID uuid.UUID         `json:"attempt_id" validate:"required"`
	Answers   []SubmitAnswerDTO `json:"answers" validate:"dive"`
}

// ===== Response =====

type ContestVoucherBriefDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ContestVoucherAdminDTO struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

type ContestPrizeDTO struct {
	ID               uuid.UUID               `json:"id"`
	RankFrom         int                     `json:"rank_from"`
	RankTo           int                     `json:"rank_to"`
	GrantCertificate bool                    `json:"grant_certificate"`
	Voucher          *ContestVoucherBriefDTO `json:"voucher"`
}

type ContestPrizeAdminDTO struct {
	ID               uuid.UUID               `json:"id"`
	RankFrom         int                     `json:"rank_from"`
	RankTo           int                     `json:"rank_to"`
	GrantCertificate bool                    `json:"grant_certificate"`
	Voucher          *ContestVoucherAdminDTO `json:"voucher"`
}

type ContestCourseBriefDTO struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Slug  string    `json:"slug"`
}

// ContestBaseDTO — phần chung của Summary/Manage; không có prizes vì 2 dạng khác kiểu voucher.
type ContestBaseDTO struct {
	ID               uuid.UUID              `json:"id"`
	Slug             string                 `json:"slug"`
	Title            string                 `json:"title"`
	Description      *string                `json:"description"`
	BannerURL        *string                `json:"banner_url"`
	Type             string                 `json:"type"`
	Status           string                 `json:"status"`
	Phase            string                 `json:"phase"`
	StartTime        time.Time              `json:"start_time"`
	EndTime          time.Time              `json:"end_time"`
	DurationMinutes  int                    `json:"duration_minutes"`
	MaxParticipants  int                    `json:"max_participants"`
	ParticipantCount int                    `json:"participant_count"`
	IsPublic         bool                   `json:"is_public"`
	Course           *ContestCourseBriefDTO `json:"course"`
	QuestionCount    int                    `json:"question_count"`
	TotalPoints      decimal.Decimal        `json:"total_points"`
	HasVoucherPrize  bool                   `json:"has_voucher_prize"`
	CreatorName      string                 `json:"creator_name"`
	FinalizedAt      *time.Time             `json:"finalized_at"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type ContestSummaryDTO struct {
	ContestBaseDTO
	Prizes []ContestPrizeDTO `json:"prizes"`
}

type ContestAwardBriefDTO struct {
	CertificateNumber *string                 `json:"certificate_number"`
	Voucher           *ContestVoucherAdminDTO `json:"voucher"`
}

type MyParticipationDTO struct {
	JoinedAt         time.Time             `json:"joined_at"`
	AttemptID        *uuid.UUID            `json:"attempt_id"`
	AttemptStatus    string                `json:"attempt_status"`
	StartedAt        *time.Time            `json:"started_at"`
	DeadlineAt       *time.Time            `json:"deadline_at"`
	SubmittedAt      *time.Time            `json:"submitted_at"`
	Score            *decimal.Decimal      `json:"score"`
	TotalPoints      *decimal.Decimal      `json:"total_points"`
	Percentage       *decimal.Decimal      `json:"percentage"`
	TimeSpentSeconds *int                  `json:"time_spent_seconds"`
	Rank             *int                  `json:"rank"`
	Award            *ContestAwardBriefDTO `json:"award"`
}

type ContestMySummaryDTO struct {
	ContestSummaryDTO
	MyParticipation *MyParticipationDTO `json:"my_participation"`
}

type ContestViewerDTO struct {
	CanJoin         bool                `json:"can_join"`
	JoinBlockReason *string             `json:"join_block_reason"`
	MyParticipation *MyParticipationDTO `json:"my_participation"`
}

type ContestDetailDTO struct {
	ContestSummaryDTO
	ServerTime               time.Time        `json:"server_time"`
	CertificateMinPercentage *decimal.Decimal `json:"certificate_min_percentage"`
	Viewer                   ContestViewerDTO `json:"viewer"`
}

type ContestQuizBriefDTO struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	QuestionCount int       `json:"question_count"`
}

type ContestManageDTO struct {
	ContestBaseDTO
	Prizes                   []ContestPrizeAdminDTO `json:"prizes"`
	Quiz                     *ContestQuizBriefDTO   `json:"quiz"`
	CourseID                 *uuid.UUID             `json:"course_id"`
	CertificateMinPercentage *decimal.Decimal       `json:"certificate_min_percentage"`
	SubmittedAt              *time.Time             `json:"submitted_at"`
	ReviewedAt               *time.Time             `json:"reviewed_at"`
	RejectReason             *string                `json:"reject_reason"`
	CancelReason             *string                `json:"cancel_reason"`
	CreatedBy                uuid.UUID              `json:"created_by"`
	CreatorEmail             string                 `json:"creator_email"`
}

type ContestQuizOptionDTO struct {
	ID            uuid.UUID       `json:"id"`
	Title         string          `json:"title"`
	QuestionCount int             `json:"question_count"`
	TotalPoints   decimal.Decimal `json:"total_points"`
}

type ContestParticipantRowDTO struct {
	UserID           uuid.UUID        `json:"user_id"`
	UserName         string           `json:"user_name"`
	AvatarURL        *string          `json:"avatar_url"`
	JoinedAt         time.Time        `json:"joined_at"`
	AttemptStatus    string           `json:"attempt_status"`
	Score            *decimal.Decimal `json:"score"`
	Percentage       *decimal.Decimal `json:"percentage"`
	TimeSpentSeconds *int             `json:"time_spent_seconds"`
	SubmittedAt      *time.Time       `json:"submitted_at"`
	Rank             *int             `json:"rank"`
}

type ContestStartResponseDTO struct {
	AttemptID       uuid.UUID            `json:"attempt_id"`
	DeadlineAt      time.Time            `json:"deadline_at"`
	ServerTime      time.Time            `json:"server_time"`
	DurationMinutes int                  `json:"duration_minutes"`
	Questions       []AttemptQuestionDTO `json:"questions"`
}

type ContestSubmitResponseDTO struct {
	AttemptID        uuid.UUID        `json:"attempt_id"`
	Score            *decimal.Decimal `json:"score"`
	TotalPoints      *decimal.Decimal `json:"total_points"`
	Percentage       *decimal.Decimal `json:"percentage"`
	TimeSpentSeconds *int             `json:"time_spent_seconds"`
	SubmittedAt      *time.Time       `json:"submitted_at"`
}

type ContestMyResultDTO struct {
	MyParticipation    *MyParticipationDTO    `json:"my_participation"`
	AnswersAvailableAt time.Time              `json:"answers_available_at"`
	Questions          []QuizAttemptAnswerDTO `json:"questions"` // nil -> null trước khi đóng
}

type LeaderboardItemDTO struct {
	Rank             int             `json:"rank"`
	UserName         string          `json:"user_name"`
	AvatarURL        *string         `json:"avatar_url"`
	Score            decimal.Decimal `json:"score"`
	TotalPoints      decimal.Decimal `json:"total_points"`
	Percentage       decimal.Decimal `json:"percentage"`
	TimeSpentSeconds int             `json:"time_spent_seconds"`
	SubmittedAt      time.Time       `json:"submitted_at"`
	IsMe             bool            `json:"is_me"`
}

type ContestCertificateDTO struct {
	CertificateNumber string    `json:"certificate_number"`
	UserName          string    `json:"user_name"`
	ContestTitle      string    `json:"contest_title"`
	ContestSlug       string    `json:"contest_slug"`
	Rank              *int      `json:"rank"`
	IssuedAt          time.Time `json:"issued_at"`
}

type FinalizeResultDTO struct {
	ContestID        uuid.UUID `json:"contest_id"`
	FinalizedAt      time.Time `json:"finalized_at"`
	AlreadyFinalized bool      `json:"already_finalized"`
	RankedCount      int       `json:"ranked_count"`
	AwardCount       int       `json:"award_count"`
	CertificateCount int       `json:"certificate_count"`
	VoucherCount     int       `json:"voucher_count"`
	NotifiedCount    int       `json:"notified_count"`
}

// ContestPage — phân trang chung (contract §0).
type ContestPage[T any] struct {
	Items      []T   `json:"items"`
	TotalCount int64 `json:"total_count"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int   `json:"total_pages"`
}

// LeaderboardPageDTO — trang BXH + cờ finalized (contract #16).
type LeaderboardPageDTO struct {
	ContestPage[LeaderboardItemDTO]
	Finalized bool `json:"finalized"`
}
