package constants

import "fmt"

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
	PrefixPasswordReset  = "password_reset:otp"
)

// Key builders
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

