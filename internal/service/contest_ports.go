package service

// contest_ports.go — ranh giới giữa lane B2 (quiz engine + phát thưởng) và lane B1 (ContestService),
// chép NGUYÊN VĂN từ contract "Cuộc thi" §6. B2 sở hữu file này; B1 chỉ dùng.
//
// Hướng phụ thuộc: QuizService (B2) gọi ContestQuizGate do ContestService (B1) hiện thực, còn
// ContestService gọi ngược lại ContestQuizEngine (QuizService) và ContestRewardIssuer
// (ContestRewardService). Đặt interface ở đây để hai phía không import vòng và B2 build/test được
// khi B1 chưa merge (gate nil = chưa có cuộc thi nào, không khoá quiz nào).

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
)

// QuizAttemptModeContest: quiz_attempts.mode của lượt làm bài thi. Không đếm vào max_attempts
// (CountAttemptsByUserAndQuiz chỉ đếm "official") và không vào thống kê giảng viên của quiz.
const QuizAttemptModeContest = "contest"

var ErrQuizLockedByContest = errors.New("quiz is locked by a contest")           // 403 QUIZ_LOCKED_BY_CONTEST
var ErrQuizEditLockedByContest = errors.New("quiz is used by an active contest") // 409 CONTEST_QUIZ_LOCKED
var ErrVoucherUnavailableForGrant = errors.New("voucher unavailable for grant")  // 409 CONTEST_VOUCHER_UNAVAILABLE

type ContestQuizGate interface { // B1 hiện thực (ContestService), quiz_service gọi
	CheckQuizAccess(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) error
	CheckQuizEditable(ctx context.Context, quizID uuid.UUID) error
}

type ContestQuizEngine interface { // B2 hiện thực (QuizService)
	CreateContestAttemptTx(ctx context.Context, tx *gorm.DB, quizID, userID uuid.UUID, startedAt time.Time) (uuid.UUID, error)
	GetContestAttemptQuestions(ctx context.Context, quizID, attemptID uuid.UUID) ([]dto.AttemptQuestionDTO, error)
	SubmitContestAttempt(ctx context.Context, quizID, userID, attemptID uuid.UUID, answers []dto.SubmitAnswerDTO, maxTimeSpentSeconds int) (*dto.QuizAttemptResponseDTO, error)
	GetContestAttemptReview(ctx context.Context, attemptID uuid.UUID) ([]dto.QuizAttemptAnswerDTO, error)
}

type ContestAwardGrant struct {
	ContestID, UserID uuid.UUID
	ContestTitle      string
	Rank              *int
	GrantCertificate  bool
	VoucherID         *uuid.UUID
	IssuedAt          time.Time
}

type ContestAwardIssued struct {
	CertificateNumber *string
	UserVoucherID     *uuid.UUID
}

type ContestResultNotice struct {
	UserID, ContestID          uuid.UUID
	ContestTitle, ContestSlug  string
	Rank                       *int
	HasCertificate, HasVoucher bool
}

type ContestRewardIssuer interface { // B2 hiện thực (ContestRewardService)
	IssueAwardTx(ctx context.Context, tx *gorm.DB, g ContestAwardGrant) (*ContestAwardIssued, error)
	NotifyContestResults(ctx context.Context, notices []ContestResultNotice) (sent int, err error)
}
