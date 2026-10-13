package router

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
)

// F-10: mọi route wishlist phải đứng sau AuthMiddleware - khách không token nhận 401, không chạm handler.
func TestWishlistRoutes_KhachKhongTokenNhan401(t *testing.T) {
	app := fiber.New()
	SetupWishlistRoutes(app.Group("/api"), &config.Config{JWTSecret: "x"}, handler.NewWishlistHandler(nil), nil)
	for _, c := range [][2]string{
		{"GET", "/api/wishlist"}, {"POST", "/api/wishlist"}, {"GET", "/api/wishlist/ids"},
		{"GET", "/api/wishlist/check/00000000-0000-0000-0000-000000000001"},
		{"DELETE", "/api/wishlist/00000000-0000-0000-0000-000000000001"},
	} {
		res, err := app.Test(httptest.NewRequest(c[0], c[1], nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 401 {
			t.Errorf("%s %s: %d, muon 401", c[0], c[1], res.StatusCode)
		}
	}
}
