package middleware

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// SameOriginRequired chan CSRF cho mot route dung cookie authentication nhung KHONG buoc
// preflight (MED-3, review 260915). POST /api/progress nhan Content-Type "text/plain" de tuong
// thich navigator.sendBeacon (xem EnrollmentHandler.TrackProgressBeacon), nen theo dinh nghia
// CORS day la mot "simple request" — trinh duyet KHONG gui preflight OPTIONS, va middleware
// cors.New (AllowOrigins) chi tu choi o buoc doc response cho JS cua trang goi, KHONG chan duoc
// request tu buoc gui di. Route nay xac thuc bang cookie (AuthMiddleware uu tien cookie hon
// Bearer — xem auth_middleware.go), va getCookieSameSite co the tra "None" khi deploy cross-
// subdomain + https, nen mot trang la co the navigator.sendBeacon toi day va trinh duyet se tu
// dinh kem cookie that cua nan nhan — cot video_watched_seconds/status chi tang/chi tien nen gia
// tri ghi vao khong the bi doi lai bang mot request khac.
//
// Kiem tra thu cong header Origin/Referer thay the vai tro cua preflight: ca hai deu do CHINH
// TRINH DUYET dat, khong the bi ghi de tu JavaScript cua trang goi (fetch/sendBeacon deu tu choi
// set lai header nay).
func SameOriginRequired(allowedOrigins string) fiber.Handler {
	allowed := make(map[string]bool)
	allowAll := false
	for _, o := range strings.Split(allowedOrigins, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			allowAll = true
			continue
		}
		allowed[o] = true
	}

	return func(c *fiber.Ctx) error {
		// ALLOWED_ORIGINS=* nghia la CORS da mo hoan toan o tang khac (vd moi truong dev); khong
		// con danh sach nao de doi chieu o day.
		if allowAll {
			return c.Next()
		}

		origin := c.Get("Origin")
		if origin == "" {
			// sendBeacon tu trang cung origin doi khi khong dat Origin nhung van dat Referer —
			// rut scheme+host tu Referer lam origin thay the.
			if referer := c.Get("Referer"); referer != "" {
				if u, err := url.Parse(referer); err == nil && u.Scheme != "" && u.Host != "" {
					origin = u.Scheme + "://" + u.Host
				}
			}
		}

		if origin == "" || !allowed[origin] {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"message": "Forbidden",
				"error":   "request origin not allowed",
			})
		}

		return c.Next()
	}
}
