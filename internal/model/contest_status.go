package model

import "time"

// MVP "Cuộc thi" (28/09/2026) — NGUỒN SỰ THẬT DUY NHẤT cho các giá trị enum của cuộc thi, cùng
// khuôn course_status.go:
//   - RunPostMigrations sinh CHECK chk_contests_status từ ContestStatuses (buildCheckConstraintSQL).
//   - Tag `check:` của Contest.Status là literal tĩnh; contest_status_test.go đối chiếu 2 chiều.
// Contract: plans/260927-2055-role-based-ux-qa/contest-feature/contract.md §1.1.

// Trạng thái LƯU DB (contests.status).
const (
	ContestStatusDraft         = "DRAFT"
	ContestStatusPendingReview = "PENDING_REVIEW"
	ContestStatusPublished     = "PUBLISHED"
	ContestStatusRejected      = "REJECTED"
	ContestStatusCancelled     = "CANCELLED"
)

var ContestStatuses = []string{
	ContestStatusDraft, ContestStatusPendingReview, ContestStatusPublished, ContestStatusRejected, ContestStatusCancelled,
}

// Phase TÍNH lúc đọc (không lưu) — 4 giá trị đầu trùng status, 4 giá trị sau chỉ có khi PUBLISHED.
const (
	ContestPhaseDraft         = ContestStatusDraft
	ContestPhasePendingReview = ContestStatusPendingReview
	ContestPhaseRejected      = ContestStatusRejected
	ContestPhaseCancelled     = ContestStatusCancelled
	ContestPhaseUpcoming      = "UPCOMING"
	ContestPhaseActive        = "ACTIVE"
	ContestPhaseEnded         = "ENDED"
	ContestPhaseFinalized     = "FINALIZED"
)

var ContestPhases = []string{
	ContestPhaseDraft, ContestPhasePendingReview, ContestPhaseRejected, ContestPhaseCancelled,
	ContestPhaseUpcoming, ContestPhaseActive, ContestPhaseEnded, ContestPhaseFinalized,
}

// Trạng thái bài làm của thí sinh (tính từ quiz_attempts, không lưu).
const (
	ContestAttemptNotStarted = "NOT_STARTED"
	ContestAttemptInProgress = "IN_PROGRESS"
	ContestAttemptSubmitted  = "SUBMITTED"
	ContestAttemptExpired    = "EXPIRED"
)

var ContestAttemptStatuses = []string{
	ContestAttemptNotStarted, ContestAttemptInProgress, ContestAttemptSubmitted, ContestAttemptExpired,
}

// Lý do người xem không đăng ký được (viewer.join_block_reason).
const (
	ContestJoinBlockLoginRequired  = "LOGIN_REQUIRED"
	ContestJoinBlockRoleNotAllowed = "ROLE_NOT_ALLOWED"
	ContestJoinBlockOwner          = "OWNER"
	ContestJoinBlockCourseRequired = "COURSE_REQUIRED"
	ContestJoinBlockFull           = "FULL"
	ContestJoinBlockClosed         = "CLOSED"
	ContestJoinBlockAlreadyJoined  = "ALREADY_JOINED"
)

var ContestJoinBlockReasons = []string{
	ContestJoinBlockLoginRequired, ContestJoinBlockRoleNotAllowed, ContestJoinBlockOwner,
	ContestJoinBlockCourseRequired, ContestJoinBlockFull, ContestJoinBlockClosed, ContestJoinBlockAlreadyJoined,
}

const (
	// ContestSubmitGraceSeconds: bù độ trễ mạng khi web tự nộp lúc đồng hồ về 0.
	ContestSubmitGraceSeconds = 30
	// ContestFinalizeDelaySeconds: chờ hết cửa sổ nộp muộn (grace) rồi mới cho chốt.
	ContestFinalizeDelaySeconds = 60
	ContestMaxPrizes            = 10
	ContestMaxPrizeRank         = 100
	ContestReasonMaxLen         = 1000
)

// ContestPhaseAt tính phase theo contract §3.1. Hàm thuần (không đọc đồng hồ) để test được mọi
// mốc thời gian; nơi gọi truyền `now` của request.
func ContestPhaseAt(status string, startTime, endTime time.Time, finalizedAt *time.Time, now time.Time) string {
	if status != ContestStatusPublished {
		return status
	}
	if finalizedAt != nil {
		return ContestPhaseFinalized
	}
	if now.Before(startTime) {
		return ContestPhaseUpcoming
	}
	if now.Before(endTime) {
		return ContestPhaseActive
	}
	return ContestPhaseEnded
}

// Phase là dạng tiện dụng của ContestPhaseAt trên một Contest đã nạp.
func (c *Contest) Phase(now time.Time) string {
	return ContestPhaseAt(c.Status, c.StartTime, c.EndTime, c.FinalizedAt, now)
}

// AnswersAvailableAt: mốc mở đáp án, kết quả chi tiết và bảng xếp hạng công khai. Bằng end_time
// cộng ĐÚNG ân hạn nộp bài: trong ân hạn, người bắt đầu sát giờ vẫn còn nộp được, nên mở đáp án
// lúc end_time sẽ cho người đã nộp chuyển đáp án cho họ (review PR #82, ĐÍNH CHÍNH 2).
func (c *Contest) AnswersAvailableAt() time.Time {
	return c.EndTime.Add(ContestSubmitGraceSeconds * time.Second)
}
