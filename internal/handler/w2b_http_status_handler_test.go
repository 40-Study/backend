package handler

// W2-B (review R3 MINOR 5): ghim mã HTTP mà các lỗi nghiệp vụ lịch / thống kê / xếp hạng phải trả. Trước đây
// bỏ ánh xạ ở scheduleFail, requireAnalyticsAuthErr hay GetLeaderboard mà các test vẫn xanh.
//   - buổi trùng giờ 409, giờ kết thúc không sau giờ bắt đầu 400 (cả lịch lặp lẫn buổi);
//   - thống kê buổi live không tồn tại 404; bảng xếp hạng của lớp không xem được 404;
//   - sessions_from/sessions_to của thời khoá biểu: sai định dạng hoặc thiếu một đầu 400, đủ thì gọi service
//     bản có buổi, khoảng không hợp lệ 400.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

func TestW2B_ScheduleHandler_OverlapIs409TimeOrderIs400(t *testing.T) {
	actor := uuid.New()
	class, id := uuid.NewString(), uuid.NewString()
	routes := []struct{ method, path, body string }{
		{"POST", "/classes/" + class + "/schedules/", `{"day_of_week":1,"start_time":"08:00","end_time":"09:00","effective_from":"2026-01-01"}`},
		{"PUT", "/classes/" + class + "/schedules/" + id, `{"end_time":"07:00"}`},
		{"POST", "/classes/" + class + "/sessions/", `{"date":"2026-01-01","start_time":"08:00","end_time":"09:00"}`},
		{"PUT", "/classes/" + class + "/sessions/" + id, `{"status":"scheduled"}`},
		{"POST", "/classes/" + class + "/sessions/generate", `{"start_date":"2026-01-01","end_date":"2026-01-07"}`},
	}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"trùng giờ", service.ErrSessionOverlap, fiber.StatusConflict},
		{"giờ kết thúc không sau giờ bắt đầu", service.ErrSessionTimeOrder, fiber.StatusBadRequest},
	} {
		for _, r := range routes {
			t.Run(tc.name+" "+r.method+" "+r.path, func(t *testing.T) {
				svc := &s5ScheduleSvc{err: tc.err}
				app := fiber.New()
				app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", actor); return c.Next() })
				h := NewScheduleHandler(svc, nil)
				cs := app.Group("/classes/:classId/schedules")
				cs.Post("/", h.CreateSchedule)
				cs.Put("/:id", h.UpdateSchedule)
				se := app.Group("/classes/:classId/sessions")
				se.Post("/", h.CreateSession)
				se.Put("/:id", h.UpdateSession)
				se.Post("/generate", h.GenerateSessions)

				req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
				req.Header.Set("Content-Type", "application/json")
				res, err := app.Test(req, -1)
				if err != nil {
					t.Fatal(err)
				}
				if res.StatusCode != tc.want {
					t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
				}
			})
		}
	}
}

// Các route lịch đều có fallback 400 nên test theo route không phân biệt được ánh xạ ErrSessionTimeOrder->400 với fallback.
// Gọi thẳng scheduleFail với fallback 500: chỉ ánh xạ thật mới ra 400/409.
func TestW2B_ScheduleFail_MapsBusinessErrorsRegardlessOfFallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"giờ kết thúc không sau giờ bắt đầu", service.ErrSessionTimeOrder, fiber.StatusBadRequest},
		{"trùng giờ", service.ErrSessionOverlap, fiber.StatusConflict},
		{"lỗi lạ giữ fallback", errors.New("boom"), fiber.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/x", func(c *fiber.Ctx) error {
				return scheduleFail(c, fmt.Errorf("lô: %w", tc.err), "Failed", fiber.StatusInternalServerError)
			})
			res, err := app.Test(httptest.NewRequest("GET", "/x", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.want {
				t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
			}
		})
	}
}

type w2bAnalyticsSvc struct {
	service.AnalyticsServiceInterface
	err error
}

func (s *w2bAnalyticsSvc) GetLivestreamAnalytics(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.AnalyticsResponseDTO, error) {
	return nil, s.err
}

func TestW2B_AnalyticsHandler_LivestreamSessionMissingIs404(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"buổi live không tồn tại", service.ErrSessionNotFound, fiber.StatusNotFound},
		{"không phải chủ buổi", service.ErrNotAnalyticsOwner, fiber.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
			app.Get("/analytics/livestream/:sessionId", NewAnalyticsHandler(&w2bAnalyticsSvc{err: tc.err}, nil).GetLivestreamAnalytics)
			res, err := app.Test(httptest.NewRequest("GET", "/analytics/livestream/"+uuid.NewString(), nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.want {
				t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
			}
		})
	}
}

type w2bLeaderboardSvc struct {
	service.LeaderboardServiceInterface
	err error
}

func (s *w2bLeaderboardSvc) GetLeaderboard(context.Context, string, int, *service.LeaderboardViewer, *uuid.UUID) (*dto.LeaderboardResponse, error) {
	return nil, s.err
}

func TestW2B_LeaderboardHandler_ClassStatusCodes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		err   error
		want  int
	}{
		{"lớp không xem được hoặc không tồn tại", "?class_id=" + uuid.NewString(), service.ErrLeaderboardClassNotFound, fiber.StatusNotFound},
		{"class_id không phải UUID", "?class_id=abc", nil, fiber.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/leaderboard", NewLeaderboardHandler(&w2bLeaderboardSvc{err: tc.err}, nil).GetLeaderboard)
			res, err := app.Test(httptest.NewRequest("GET", "/leaderboard"+tc.query, nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.want {
				t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
			}
		})
	}
}

func (s *w2bLeaderboardSvc) ListMyClassBoards(context.Context, uuid.UUID) ([]dto.LeaderboardClassDTO, error) {
	return []dto.LeaderboardClassDTO{{ID: uuid.MustParse("3f2504e0-4f89-41d3-9a0c-0305e82c3301"), Name: "Lớp A"}}, s.err
}

func TestW2B_LeaderboardHandler_MyClassBoards(t *testing.T) {
	t.Run("chưa đăng nhập 401", func(t *testing.T) {
		app := fiber.New()
		app.Get("/leaderboard/classes", NewLeaderboardHandler(&w2bLeaderboardSvc{}, nil).GetMyClassBoards)
		res, err := app.Test(httptest.NewRequest("GET", "/leaderboard/classes", nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("status=%d, muốn 401", res.StatusCode)
		}
	})
	t.Run("đăng nhập trả danh sách lớp", func(t *testing.T) {
		app := mountWithCaller("GET", "/leaderboard/classes", uuid.New(), NewLeaderboardHandler(&w2bLeaderboardSvc{}, nil).GetMyClassBoards)
		res, err := app.Test(httptest.NewRequest("GET", "/leaderboard/classes", nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != fiber.StatusOK || !strings.Contains(string(body), "Lớp A") {
			t.Fatalf("status=%d body=%s", res.StatusCode, body)
		}
	})
}

type w2bTimetableSvc struct {
	service.ScheduleServiceInterface
	err         error
	withSession bool
	from, to    time.Time
}

func (s *w2bTimetableSvc) GetMyTimetable(context.Context, uuid.UUID, string) (*dto.TimetableResponseDTO, error) {
	return &dto.TimetableResponseDTO{}, nil
}

func (s *w2bTimetableSvc) GetMyTimetableWithSessions(_ context.Context, _ uuid.UUID, _ string, from, to time.Time) (*dto.TimetableResponseDTO, error) {
	s.withSession, s.from, s.to = true, from, to
	return &dto.TimetableResponseDTO{}, s.err
}

func TestW2B_ScheduleHandler_MyTimetableSessionsRange(t *testing.T) {
	for _, tc := range []struct {
		name        string
		query       string
		svcErr      error
		want        int
		wantSession bool
	}{
		{"không gửi khoảng: chỉ lịch lặp", "", nil, fiber.StatusOK, false},
		{"đủ hai đầu: gọi bản có buổi", "?sessions_from=2031-03-01&sessions_to=2031-03-31", nil, fiber.StatusOK, true},
		{"thiếu sessions_to", "?sessions_from=2031-03-01", nil, fiber.StatusBadRequest, false},
		{"thiếu sessions_from", "?sessions_to=2031-03-31", nil, fiber.StatusBadRequest, false},
		{"sai định dạng", "?sessions_from=01/03/2031&sessions_to=2031-03-31", nil, fiber.StatusBadRequest, false},
		{"khoảng không hợp lệ (service)", "?sessions_from=2031-03-31&sessions_to=2031-03-01", service.ErrTimetableRangeInvalid, fiber.StatusBadRequest, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &w2bTimetableSvc{err: tc.svcErr}
			app := mountWithCaller("GET", "/timetable/me", uuid.New(), NewScheduleHandler(svc, nil).GetMyTimetable)
			res, err := app.Test(httptest.NewRequest("GET", "/timetable/me"+tc.query, nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.want {
				t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
			}
			if svc.withSession != tc.wantSession {
				t.Fatalf("gọi bản có buổi=%v, muốn %v", svc.withSession, tc.wantSession)
			}
			if tc.want == fiber.StatusOK && tc.wantSession && (svc.from.Format("2006-01-02") != "2031-03-01" || svc.to.Format("2006-01-02") != "2031-03-31") {
				t.Fatalf("khoảng ngày truyền xuống service sai: %v - %v", svc.from, svc.to)
			}
		})
	}
}
