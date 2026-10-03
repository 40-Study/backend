package handler

// Lỗi: web gửi ?period_type=weekly|monthly|... nhưng handler chỉ đọc `period` (mặc định all_time),
// nên mọi tab bảng xếp hạng đều ra all_time. Test khoá lại: handler nhận cả `period` lẫn
// `period_type`, rỗng → all_time, giá trị ngoài enum → 400 (không gọi service, không lùi về all_time).

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type stubLeaderboardService struct {
	calls      int
	periodType string
}

func (s *stubLeaderboardService) GetLeaderboard(ctx context.Context, periodType string, limit int, _ *service.LeaderboardViewer, _ *uuid.UUID) (*dto.LeaderboardResponse, error) {
	s.calls++
	s.periodType = periodType
	return &dto.LeaderboardResponse{PeriodType: periodType}, nil
}

func (s *stubLeaderboardService) GetMyRank(ctx context.Context, userID uuid.UUID, periodType string) (*dto.MyRankResponse, error) {
	s.calls++
	s.periodType = periodType
	return &dto.MyRankResponse{PeriodType: periodType}, nil
}

func TestLeaderboardHandler_PeriodQuery(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantStatus int
		wantPeriod string // "" = service không được gọi
	}{
		{"period_type weekly (web gửi)", "?period_type=weekly", fiber.StatusOK, "weekly"},
		{"period monthly (tên cũ)", "?period=monthly", fiber.StatusOK, "monthly"},
		{"không tham số → all_time", "", fiber.StatusOK, "all_time"},
		{"cả hai → ưu tiên period", "?period=monthly&period_type=weekly", fiber.StatusOK, "monthly"},
		{"period rỗng → dùng period_type", "?period=&period_type=weekly", fiber.StatusOK, "weekly"},
		{"period_type sai → 400", "?period_type=week", fiber.StatusBadRequest, ""},
		{"period sai → 400", "?period=yearly", fiber.StatusBadRequest, ""},
		{"daily (tab Ngày của web) chưa hỗ trợ → 400", "?period_type=daily", fiber.StatusBadRequest, ""},
	}

	endpoints := []struct {
		name  string
		path  string
		mount func(h *LeaderboardHandler) *fiber.App
	}{
		{"GetLeaderboard", "/leaderboard", func(h *LeaderboardHandler) *fiber.App {
			app := fiber.New()
			app.Get("/leaderboard", h.GetLeaderboard)
			return app
		}},
		{"GetMyRank", "/leaderboard/me", func(h *LeaderboardHandler) *fiber.App {
			return mountWithCaller("GET", "/leaderboard/me", uuid.New(), h.GetMyRank)
		}},
	}

	for _, ep := range endpoints {
		for _, tc := range cases {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				svc := &stubLeaderboardService{}
				app := ep.mount(NewLeaderboardHandler(svc, nil))

				resp, err := app.Test(httptest.NewRequest("GET", ep.path+tc.query, nil))
				if err != nil {
					t.Fatalf("app.Test loi: %v", err)
				}
				if resp.StatusCode != tc.wantStatus {
					t.Fatalf("status = %d, muon %d", resp.StatusCode, tc.wantStatus)
				}
				if tc.wantPeriod == "" {
					if svc.calls != 0 {
						t.Fatalf("gia tri sai van goi service voi period %q", svc.periodType)
					}
					return
				}
				if svc.periodType != tc.wantPeriod {
					t.Fatalf("service nhan period %q, muon %q", svc.periodType, tc.wantPeriod)
				}
			})
		}
	}
}
