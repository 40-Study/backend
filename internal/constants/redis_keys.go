package constants

import (
	"fmt"

	"github.com/google/uuid"
)

// Redis key prefixes for auth
const (
	PrefixUserVersion  = "auth:user_version"
	PrefixSession      = "auth:session"
	PrefixRefresh      = "auth:refresh"
	PrefixPendingLogin = "pending_login"
	PrefixUserCache    = "user_cache"
	PrefixLoginAttempts = "login_attempts"
	PrefixLoginLocked   = "login_locked"
	PrefixRegisterOTP   = "register:otp"
	PrefixPasswordReset = "password_reset:otp"
	// PrefixAccountLocked (Phase 1 quản lý người dùng): marker riêng cho lý do "tài khoản bị
	// admin khoá" — user_version cũng bump khi đổi mật khẩu/đăng xuất nơi khác, nên
	// AuthMiddleware không thể suy ra lý do CHỈ từ user_version lệch. Set/Del cùng lúc với
	// is_active trong UserAdminService, không TTL (tồn tại tới khi mở khoá).
	PrefixAccountLocked = "auth:account_locked"
    PrefixParentInviteRateLimit = "parent_invite:rate" // key lưu số lần gửi lời mời phụ huynh trong 24 giờ của mỗi học sinh, format: parent_invite:rate:{studentID}

	// Schedule & Timetable
	PrefixScheduleCache  = "schedules:class"
	PrefixSessionCache   = "sessions:class"
	PrefixTimetableCache = "timetable:user"

	// Quiz
	PrefixQuizCache    = "quiz"
	PrefixAttemptCache = "quiz_attempt"

	// Grade
	PrefixGradeBookCache = "gradebook:class"

	// Exercise
	PrefixExerciseCache = "exercise"

	// Review
	PrefixReviewRating = "course_rating"

	// Certificate
	PrefixCertVerify = "cert_verify"
)

func KeyParentInviteRateLimit(studentID uuid.UUID) string {
    return fmt.Sprintf("%s:%s", PrefixParentInviteRateLimit, studentID)
}

func KeyUserVersion(userID string) string {
	return fmt.Sprintf("%s:%s", PrefixUserVersion, userID)
}

func KeySession(userID string) string {
	return fmt.Sprintf("%s:%s", PrefixSession, userID)
}

func KeyRefresh(userID string) string {
	return fmt.Sprintf("%s:%s", PrefixRefresh, userID)
}

func KeyPendingLogin(sessionToken string) string {
	return fmt.Sprintf("%s:%s", PrefixPendingLogin, sessionToken)
}

func KeyUserCache(userID string) string {
	return fmt.Sprintf("%s:%s", PrefixUserCache, userID)
}

func KeyLoginAttempts(email string) string {
	return fmt.Sprintf("%s:%s", PrefixLoginAttempts, email)
}

func KeyLoginLocked(email string) string {
	return fmt.Sprintf("%s:%s", PrefixLoginLocked, email)
}

func KeyRegisterOTP(email string) string {
	return fmt.Sprintf("%s:%s", PrefixRegisterOTP, email)
}

func KeyPasswordReset(userID string) string {
	return fmt.Sprintf("%s:%s", PrefixPasswordReset, userID)
}

func KeyAccountLocked(userID string) string {
	return fmt.Sprintf("%s:%s", PrefixAccountLocked, userID)
}

