package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupAuthRoutes(api fiber.Router, cfg *config.Config, authHandler *handler.AuthHandler, oauthHandler *handler.OAuthHandler, redis *redis.Client) {
	auth := api.Group("/auth")

	// Rate limiters for security-sensitive endpoints
	authRateLimiter := middleware.AuthRateLimiter(redis)
	otpRateLimiter := middleware.OTPRateLimiter(redis)

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
	auth.Post("/register", authRateLimiter, authHandler.Register)
	auth.Post("/login", authRateLimiter, authHandler.Login)
	// M-03 (audit 260909): select-role/refresh-token trước đây không rate-limit dù chạm
	// Redis/DB và cấp token — select-role còn là bề mặt khai thác của C-01.
	auth.Post("/select-role", authRateLimiter, authHandler.SelectRole)
	// select-org là bước 3 của luồng đăng nhập (session_token, chưa có access token) —
	// web/src/lib/meet/auth.ts gọi route này sau select-role. Cùng bề mặt khai thác như
	// select-role nên cũng đi qua authRateLimiter.
	auth.Post("/select-org", authRateLimiter, authHandler.SelectOrg)
	auth.Get("/system-roles", authHandler.GetSystemRoleOptions)
	auth.Post("/reset-password/request", otpRateLimiter, authHandler.RequestPasswordReset)
	auth.Post("/reset-password", authRateLimiter, authHandler.ResetPassword)
	auth.Post("/refresh-token", authRateLimiter, authHandler.RefreshToken)

	// Protected routes
	auth.Use(middleware.AuthMiddleware(cfg, redis))

	// Role management
	auth.Get("/my-roles", authHandler.GetMyRoles)
	auth.Post("/switch-role", authHandler.SwitchRole)

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
