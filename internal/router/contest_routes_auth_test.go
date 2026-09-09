package router

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/service"
)

// fakeContestServiceForRouteTest (C-02, review vòng 5→6) — chỉ implement 2 method mà
// GetContest/GetMyContests thật sự gọi tới, đủ để test router THẬT (Fiber app.Test, không phải
// quét text tĩnh) mà không cần DB/service thật. Trước đây review vòng 5 đo bằng probe dựng
// riêng rồi XÓA đi — test này giữ lại vĩnh viễn làm regression test.
type fakeContestServiceForRouteTest struct {
	service.ContestServiceInterface
}

func (f *fakeContestServiceForRouteTest) GetContestBySlug(ctx context.Context, slug string, userID *uuid.UUID) (*dto.ContestResponse, error) {
	return &dto.ContestResponse{Slug: slug}, nil
}

func (f *fakeContestServiceForRouteTest) GetMyContests(ctx context.Context, userID uuid.UUID) ([]dto.ContestResponse, error) {
	return []dto.ContestResponse{}, nil
}

// TestContestRoutes_SlugPublicMeRequiresAuth (C-02, review vòng 5→6) — dựng Fiber app THẬT,
// gọi SetupContestRoutes THẬT, gửi request THẬT không kèm token:
//   - GET /api/contests/:slug  -> KHÔNG được là 401 (route công khai, dùng để tra cứu chi tiết
//     cuộc thi không cần đăng nhập).
//   - GET /api/contests/me     -> PHẢI là 401 (route riêng tư, yêu cầu access token).
//
// Đây chính là bug C-02: bản vá vòng 5 (authed := contests.Group(""); authed.Use(auth)) làm
// "/:slug" bị 401 dù đăng ký qua biến "contests" (public) vì Use() áp theo TIỀN TỐ, không theo
// biến Go — mutation dựng lại pattern group+Use() này ở test dưới phải làm test đỏ.
func TestContestRoutes_SlugPublicMeRequiresAuth(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	h := handler.NewContestHandler(&fakeContestServiceForRouteTest{})
	SetupContestRoutes(api, &config.Config{}, h, nil)

	t.Run("GET /contests/:slug công khai, không token vẫn KHÔNG bị 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/contests/some-contest-slug", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test lỗi: %v", err)
		}
		if resp.StatusCode == fiber.StatusUnauthorized {
			t.Errorf("expected /:slug PUBLIC (không 401 dù thiếu token), nhận 401 — C-02 tái phát")
		}
	})

	t.Run("GET /contests/me thiếu token PHẢI bị 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/contests/me", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test lỗi: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected 401 cho /me thiếu token, nhận %d — I-04 tái phát (mất auth)", resp.StatusCode)
		}
	})
}
