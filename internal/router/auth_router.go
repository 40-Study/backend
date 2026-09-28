package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupAuthRoutes(api fiber.Router, cfg *config.Config, authHandler *handler.AuthHandler, oauthHandler *handler.OAuthHandler, redis *redis.Client, permChecker *middleware.PermissionChecker) {
	auth := api.Group("/auth")

	// Rate limiters for security-sensitive endpoints.
	// authRateLimiter (5/phút/IP): CHỈ cho bề mặt đoán mật khẩu/OTP thật — /login, /register,
	// /reset-password. postAuthRateLimiter (30/phút/IP, bucket "rate:post-auth"): cho mọi route
	// chỉ chạy được SAU KHI đã có credential/pending-token/access-token hợp lệ — select-role,
	// select-org, refresh-token — xem lý do đầy đủ tại middleware.WideAuthRateLimiter (S-P1-2,
	// QA 260927).
	//
	// trustedProxies (review vòng 2, PR #69 — MAJOR): key của mọi limiter theo IP giờ đi qua
	// middleware.ClientIP(), KHÔNG dùng c.IP() mặc định của Fiber (Fiber mặc định lấy IP đầu
	// tiên/bên trái nhất trong X-Forwarded-For — client tự khai được, giả mạo để né rate-limit).
	// ClientIP() duyệt XFF từ PHẢI sang trái, bỏ qua các IP thuộc TRUSTED_PROXIES — xem
	// client_ip.go cho thuật toán đầy đủ.
	trustedProxies := middleware.NewTrustedProxySet(cfg.ResolvedTrustedProxies())
	authRateLimiter := middleware.AuthRateLimiter(redis, trustedProxies)
	otpRateLimiter := middleware.OTPRateLimiter(redis, trustedProxies)
	postAuthRateLimiter := middleware.WideAuthRateLimiter(redis, "rate:post-auth", 30, trustedProxies)

	// accountLockout* (review vòng 3, PR #69 — phòng thủ chiều sâu): độc lập HOÀN TOÀN với
	// IP/X-Forwarded-For, khoá theo EMAIL trong body — vẫn chặn được brute-force kể cả khi lớp
	// rate-limit theo IP ở trên bị né hoàn toàn (client xoay X-Forwarded-For liên tục, chỉ có
	// thể xảy ra nếu cổng backend từng lộ trực tiếp không qua nginx — xem
	// middleware/account_lockout.go và mục "Vận hành" trong PR body). Mỗi route dùng KeyPrefix
	// riêng để không chia sẻ bộ đếm giữa các bề mặt khác nhau (dò mật khẩu vs dò OTP).
	accountLockoutLogin := middleware.AccountFailureLockout(redis, middleware.AccountLockoutConfig{KeyPrefix: "lockout:login"})
	accountLockoutRegister := middleware.AccountFailureLockout(redis, middleware.AccountLockoutConfig{KeyPrefix: "lockout:register"})
	accountLockoutResetPassword := middleware.AccountFailureLockout(redis, middleware.AccountLockoutConfig{KeyPrefix: "lockout:reset-password"})

	// ===== OAuth routes (public, không cần auth) =====
	// GET /auth/oauth/github          → redirect tới GitHub
	// GET /auth/oauth/github/callback  → GitHub redirect về đây
	// GET /auth/oauth/google          → redirect tới Google
	// GET /auth/oauth/google/callback  → Google redirect về đây
	// GET /auth/oauth/facebook        → redirect tới Facebook
	// GET /auth/oauth/facebook/callback → Facebook redirect về đây
	auth.Get("/oauth/:provider", oauthHandler.RedirectToProvider)
	auth.Get("/oauth/:provider/callback", oauthHandler.ProviderCallback)

	// Public routes with rate limiting
	auth.Post("/register/request", otpRateLimiter, authHandler.RequestRegister)
	auth.Post("/register", authRateLimiter, accountLockoutRegister, authHandler.Register)
	auth.Post("/login", authRateLimiter, accountLockoutLogin, authHandler.Login)
	// M-03 (audit 260909): select-role trước đây không rate-limit dù chạm Redis/DB và cấp
	// token — là bề mặt khai thác của C-01, nên PHẢI rate-limit. S-P1-2 (QA 260927, bổ sung):
	// nhưng KHÔNG dùng chung bucket 5/phút/IP với /login — select-role chỉ chạy được sau khi đã
	// qua bước xác thực mật khẩu (cầm session/pending token hợp lệ), không phải bề mặt dò mật
	// khẩu, và một lần đăng nhập trọn vẹn luôn gọi CẢ HAI (login rồi select-role) nên dùng chung
	// bucket chặt sẽ tự làm người dùng thật hết lượt đăng nhập trong 1 phút. Dùng
	// postAuthRateLimiter (30/phút/IP) — vẫn chặn được lạm dụng, không tự nghẽn luồng đăng nhập
	// thật.
	auth.Post("/select-role", postAuthRateLimiter, authHandler.SelectRole)
	auth.Get("/system-roles", authHandler.GetSystemRoleOptions)
	auth.Post("/reset-password/request", otpRateLimiter, authHandler.RequestPasswordReset)
	auth.Post("/reset-password", authRateLimiter, accountLockoutResetPassword, authHandler.ResetPassword)
	auth.Post("/refresh-token", postAuthRateLimiter, authHandler.RefreshToken)

	// Protected routes
	auth.Use(middleware.AuthMiddleware(cfg, redis))

	// Role management
	auth.Get("/my-roles", authHandler.GetMyRoles)
	// Quyen cua chinh nguoi goi (web can de hien thi/an chuc nang); khong can permission quan tri.
	if permChecker != nil {
		auth.Get("/me/permissions", permChecker.MyPermissions)
	}
	auth.Post("/switch-role", authHandler.SwitchRole)
	// select-org đổi tổ chức đang hoạt động (giữ nguyên role), cấp lại token mang active_org mới.
	//
	// BLOCKER-1 (review 260915): trước đây route này nằm ở nhóm CÔNG KHAI và dùng session_token
	// của luồng đăng nhập, nhưng SelectRole luôn hoàn tất login rồi xoá pending key nên
	// pending.SelectedRole luôn nil ⇒ mọi lời gọi trả 400, route là code chết. Nay đặt SAU
	// AuthMiddleware: danh tính lấy từ access token, không phụ thuộc pending key.
	//
	// N5 (review vong 2, 260915): route nay cham Redis/DB va cap lai token giong het select-role
	// nhung bi bo sot rate-limit khi chuyen sang nhom protected. S-P1-2 (QA 260927, bổ sung):
	// dùng postAuthRateLimiter (30/phút/IP, KHÔNG phải authRateLimiter 5/phút/IP) — route này
	// đứng SAU AuthMiddleware nên bắt buộc phải có access token thật còn hiệu lực mới gọi tới
	// được, càng không phải bề mặt dò mật khẩu; cùng lý do và cùng bucket với select-role/
	// refresh-token ở trên, để đổi tổ chức nhiều lần trong phiên làm việc không tự đụng trần.
	auth.Post("/select-org", postAuthRateLimiter, authHandler.SelectOrg)

	// Profile management
	auth.Get("/me/profiles", authHandler.GetMyProfiles)
	auth.Post("/me/profiles", authHandler.CreateProfile)
	auth.Delete("/me/profiles/:id", authHandler.DeleteProfile)

	// User info
	auth.Get("/me", authHandler.GetMe)
	auth.Put("/me", authHandler.UpdateMe)

	// Session & devices
	auth.Get("/devices", authHandler.GetAllDevices)
	auth.Post("/logout", authHandler.LogoutOneDevice)
	auth.Post("/logout-all", authHandler.LogoutAll)
	auth.Put("/change-password", authHandler.ChangePassword)

	// Account security
	auth.Delete("/me", authHandler.DeleteAccount)

	// Linked OAuth accounts
	auth.Get("/linked-accounts", oauthHandler.ListLinkedAccounts)
	auth.Delete("/linked-accounts/:provider", oauthHandler.DisconnectProvider)
}
