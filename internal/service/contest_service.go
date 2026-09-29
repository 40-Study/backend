package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ContestService — MVP "Cuộc thi" (contract plans/260927-2055-role-based-ux-qa/contest-feature/
// contract.md). Tách file theo luồng: contest_query_service.go (đọc), contest_lifecycle_service.go
// (tạo/sửa/duyệt), contest_play_service.go (tham gia/làm bài/BXH), contest_finalize_service.go
// (chốt). Chấm bài và phát thưởng do lane B2 hiện thực qua ContestQuizEngine/ContestRewardIssuer
// (contest_ports.go) — service này chỉ điều phối, không tự chấm điểm.
type ContestService struct {
	repo        *repository.ContestRepository
	engine      ContestQuizEngine
	issuer      ContestRewardIssuer
	enrollments ContestEnrollmentReader
	now         func() time.Time
}

// ContestEnrollmentReader — chỉ phần EnrollmentRepository cần cho điều kiện "phải có khoá học".
type ContestEnrollmentReader interface {
	GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error)
}

func NewContestService(
	repo *repository.ContestRepository,
	engine ContestQuizEngine,
	issuer ContestRewardIssuer,
	enrollments ContestEnrollmentReader,
) *ContestService {
	return &ContestService{repo: repo, engine: engine, issuer: issuer, enrollments: enrollments, now: time.Now}
}

// SetClock thay đồng hồ (chỉ dùng trong test để đi qua các mốc phase mà không phải chờ thật).
func (s *ContestService) SetClock(now func() time.Time) { s.now = now }

// ContestActor — người gọi. nil = khách chưa đăng nhập. IsAdmin tính ở handler bằng
// HasPermission(CONTESTS_APPROVE_ALL) (contract §0).
type ContestActor struct {
	UserID     uuid.UUID
	ActiveRole string
	IsAdmin    bool
}

func (a *ContestActor) isOwnerOf(c *model.Contest) bool { return a != nil && a.UserID == c.CreatedBy }
func (a *ContestActor) canManage(c *model.Contest) bool { return a != nil && (a.IsAdmin || a.UserID == c.CreatedBy) }
func (a *ContestActor) isStudent() bool                 { return a != nil && a.ActiveRole == "STUDENT" }

// ── Lỗi nghiệp vụ: (HTTP, code) đúng bảng contract §2.4 ────────────────────

type ContestError struct {
	Status  int
	Code    string
	Message string
	Details interface{} // tuỳ chọn, handler trả ở field "details" (vd. chi tiết voucher khi chốt)
}

func (e *ContestError) Error() string { return e.Code + ": " + e.Message }

// Is so theo Status+Code: lỗi dựng riêng kèm Details (withDetails) vẫn khớp sentinel bằng errors.Is.
func (e *ContestError) Is(target error) bool {
	t, ok := target.(*ContestError)
	return ok && t.Status == e.Status && t.Code == e.Code
}

// withDetails trả BẢN SAO của sentinel với message/details riêng — không bao giờ sửa sentinel dùng chung.
func (e *ContestError) withDetails(message string, details interface{}) *ContestError {
	return &ContestError{Status: e.Status, Code: e.Code, Message: message, Details: details}
}

func cerr(status int, code, msg string) *ContestError {
	return &ContestError{Status: status, Code: code, Message: msg}
}

var (
	ErrContestInvalidSchedule   = cerr(400, "CONTEST_INVALID_SCHEDULE", "Lịch cuộc thi không hợp lệ")
	ErrContestQuizInvalid       = cerr(400, "CONTEST_QUIZ_INVALID", "Bài trắc nghiệm không đủ điều kiện cho cuộc thi")
	ErrContestPrizesInvalid     = cerr(400, "CONTEST_PRIZES_INVALID", "Cơ cấu giải không hợp lệ")
	ErrContestReasonRequired    = cerr(400, "REASON_REQUIRED", "Vui lòng nhập lý do")
	ErrContestReasonTooLong     = cerr(400, "REASON_TOO_LONG", "Lý do tối đa 1000 ký tự")
	ErrContestForbidden         = cerr(403, "CONTEST_FORBIDDEN", "Bạn không có quyền với cuộc thi này")
	ErrContestRoleNotAllowed    = cerr(403, "CONTEST_ROLE_NOT_ALLOWED", "Chỉ học viên được tham gia cuộc thi")
	ErrContestOwnerCannotJoin   = cerr(403, "CONTEST_OWNER_CANNOT_JOIN", "Người tạo không thể tham gia cuộc thi của mình")
	ErrContestCourseRequired    = cerr(403, "CONTEST_COURSE_REQUIRED", "Cần đăng ký khoá học để tham gia")
	ErrContestVoucherAdminOnly  = cerr(403, "CONTEST_VOUCHER_ADMIN_ONLY", "Chỉ quản trị viên được gắn giải voucher")
	ErrContestNotJoined         = cerr(403, "CONTEST_NOT_JOINED", "Bạn chưa đăng ký cuộc thi này")
	ErrContestLeaderboardHidden = cerr(403, "CONTEST_LEADERBOARD_HIDDEN", "Bảng xếp hạng hiển thị sau khi cuộc thi kết thúc")
	ErrContestNotFound          = cerr(404, "CONTEST_NOT_FOUND", "Không tìm thấy cuộc thi")
	ErrContestResultNotFound    = cerr(404, "CONTEST_RESULT_NOT_FOUND", "Bạn chưa nộp bài cuộc thi này")
	ErrContestCertNotFound      = cerr(404, "CONTEST_CERTIFICATE_NOT_FOUND", "Không có chứng nhận")
	ErrContestInvalidStatus     = cerr(409, "CONTEST_INVALID_STATUS", "Trạng thái cuộc thi không cho phép thao tác này")
	ErrContestStartPassed       = cerr(409, "CONTEST_START_PASSED", "Đã qua giờ bắt đầu cuộc thi")
	ErrContestQuizInUse         = cerr(409, "CONTEST_QUIZ_IN_USE", "Bài trắc nghiệm đã gắn với cuộc thi khác")
	ErrContestClosed            = cerr(409, "CONTEST_CLOSED", "Cuộc thi đã đóng đăng ký")
	ErrContestNotActive         = cerr(409, "CONTEST_NOT_ACTIVE", "Cuộc thi không trong thời gian làm bài")
	ErrContestAlreadyJoined     = cerr(409, "CONTEST_ALREADY_JOINED", "Bạn đã đăng ký cuộc thi này")
	ErrContestFull              = cerr(409, "CONTEST_FULL", "Cuộc thi đã đủ số người tham gia")
	ErrContestAlreadySubmitted  = cerr(409, "CONTEST_ALREADY_SUBMITTED", "Bạn đã nộp bài")
	ErrContestDeadlinePassed    = cerr(409, "CONTEST_DEADLINE_PASSED", "Đã hết thời gian làm bài")
	ErrContestAttemptMismatch   = cerr(409, "CONTEST_ATTEMPT_MISMATCH", "Bài làm không thuộc cuộc thi này")
	ErrContestNotEnded          = cerr(409, "CONTEST_NOT_ENDED", "Cuộc thi chưa kết thúc")
	ErrContestVoucherUnusable   = cerr(409, "CONTEST_VOUCHER_UNAVAILABLE", "Voucher của giải không còn dùng được")
)

// ── ContestQuizGate (contract §3.2): quiz_service (B2) gọi để khoá quiz đã gắn cuộc thi ──

var _ ContestQuizGate = (*ContestService)(nil)

// CheckQuizAccess: quiz gắn cuộc thi chỉ người tạo cuộc thi và admin đọc/làm qua /api/quizzes,
// /api/attempts — học viên phải đi qua /api/contests để không lộ đáp án trước giờ.
func (s *ContestService) CheckQuizAccess(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	c, err := s.repo.FindByQuizID(ctx, quizID)
	if err != nil {
		return err
	}
	if c == nil || isAdmin || c.CreatedBy == userID {
		return nil
	}
	return ErrQuizLockedByContest
}

// CheckQuizEditable: đề đã gửi duyệt/đang chạy/đã huỷ thì KHÔNG ai sửa được (kể cả admin) —
// sửa đề sau khi duyệt sẽ làm lệch điểm giữa các thí sinh.
func (s *ContestService) CheckQuizEditable(ctx context.Context, quizID uuid.UUID) error {
	c, err := s.repo.FindByQuizID(ctx, quizID)
	if err != nil || c == nil {
		return err
	}
	if isOneOf(c.Status, []string{model.ContestStatusPendingReview, model.ContestStatusPublished, model.ContestStatusCancelled}) {
		return ErrQuizEditLockedByContest
	}
	return nil
}

// ── Tiện ích chung ─────────────────────────────────────────────────────────

func normalizePage(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}
	return page, limit
}

// loadForManage — chủ hoặc admin; người khác nhận notFoundErr (ẩn sự tồn tại) hoặc 403.
func (s *ContestService) loadForManage(ctx context.Context, id uuid.UUID, actor *ContestActor, hide bool) (*model.Contest, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrContestNotFound
	}
	if !actor.canManage(c) {
		if hide {
			return nil, ErrContestNotFound
		}
		return nil, ErrContestForbidden
	}
	return c, nil
}

// normalizeContestReason dùng lại quy tắc NormalizeReviewReason (trim, 1..1000) với code riêng.
func normalizeContestReason(raw string) (string, error) {
	r, err := NormalizeReviewReason(raw)
	switch {
	case errors.Is(err, ErrReviewReasonRequired):
		return "", ErrContestReasonRequired
	case errors.Is(err, ErrReviewReasonTooLong):
		return "", ErrContestReasonTooLong
	}
	return r, err
}
