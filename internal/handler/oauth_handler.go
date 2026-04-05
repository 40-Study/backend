package handler

import (
	"net/url"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type OAuthHandler struct {
	oauthService *service.OAuthService
	cfg          *config.Config
}

func NewOAuthHandler(oauthService *service.OAuthService, cfg *config.Config) *OAuthHandler {
	return &OAuthHandler{
		oauthService: oauthService,
		cfg:          cfg,
	}
}

// RedirectToProvider xử lý GET /auth/oauth/:provider
// Tạo URL redirect tới provider (GitHub/Google/Facebook) rồi 302 redirect browser
// Frontend gọi bằng cách: window.location.href = "/api/auth/oauth/github?device_id=...&device_name=...&os=..."
func (h *OAuthHandler) RedirectToProvider(c *fiber.Ctx) error {
	providerName := c.Params("provider") // "github", "google", "facebook"

	// Lấy device info từ query params (frontend truyền qua URL)
	deviceInfo := &dto.DeviceInfoDTO{
		DeviceID:   c.Query("device_id"),
		DeviceName: c.Query("device_name"),
		OS:         c.Query("os"),
	}

	// Gọi service tạo auth URL (bao gồm tạo state + lưu Redis)
	authURL, err := h.oauthService.GetAuthURL(c.Context(), providerName, deviceInfo)
	if err != nil {
		// Nếu provider không hỗ trợ hoặc lỗi khác → redirect về frontend với error
		return c.Redirect(h.cfg.FrontendURL+"/login?error="+url.QueryEscape(err.Error()), fiber.StatusTemporaryRedirect)
	}

	// 302 redirect tới provider (GitHub/Google/Facebook)
	return c.Redirect(authURL, fiber.StatusTemporaryRedirect)
}

// ProviderCallback xử lý GET /auth/oauth/:provider/callback
// Đây là endpoint mà provider (GitHub/Google/Facebook) redirect về sau khi user authorize
// Browser sẽ navigate trực tiếp tới URL này, KHÔNG phải AJAX call
func (h *OAuthHandler) ProviderCallback(c *fiber.Ctx) error {
	providerName := c.Params("provider")
	code := c.Query("code")
	state := c.Query("state")

	// Nếu thiếu code hoặc state → redirect về frontend với error
	if code == "" || state == "" {
		errorMsg := c.Query("error_description", c.Query("error", "missing code or state"))
		return c.Redirect(h.cfg.FrontendURL+"/login?error="+url.QueryEscape(errorMsg), fiber.StatusTemporaryRedirect)
	}

	// Gọi service xử lý callback: verify state, exchange code, tìm/tạo user, login
	result, err := h.oauthService.HandleCallback(c.Context(), providerName, code, state)
	if err != nil {
		return c.Redirect(h.cfg.FrontendURL+"/login?error="+url.QueryEscape(err.Error()), fiber.StatusTemporaryRedirect)
	}

	// Nếu login thành công (completed = true, có tokens)
	if result.Completed && result.AccessToken != "" {
		// Set cookies giống auth_handler.go
		secure := os.Getenv("ENVIRONMENT") == "production"
		c.Cookie(&fiber.Cookie{
			Name:     "accessToken",
			Value:    result.AccessToken,
			Expires:  time.Now().Add(h.cfg.JWTAccessExpiration),
			HTTPOnly: true,
			Secure:   secure,
			SameSite: "Lax",
		})
		c.Cookie(&fiber.Cookie{
			Name:     "rfToken",
			Value:    result.RefreshToken,
			Expires:  time.Now().Add(h.cfg.JWTRefreshExpiration),
			HTTPOnly: true,
			Secure:   secure,
			SameSite: "Lax",
		})

		// Redirect về frontend — login xong
		return c.Redirect(h.cfg.FrontendURL+"/?oauth=success", fiber.StatusTemporaryRedirect)
	}

	// Nếu cần chọn role (multi-role user) → redirect về trang chọn role
	if result.SessionToken != "" {
		return c.Redirect(
			h.cfg.FrontendURL+"/select-role?session_token="+url.QueryEscape(result.SessionToken),
			fiber.StatusTemporaryRedirect,
		)
	}

	// Fallback — không nên xảy ra
	return c.Redirect(h.cfg.FrontendURL+"/login?error=unexpected_oauth_result", fiber.StatusTemporaryRedirect)
}
