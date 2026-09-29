package handler

// Lane S4 vòng 2: các route điểm danh của lớp truyền người gọi xuống service và ánh xạ lỗi uỷ quyền:
// lớp không xem được -> 404, bản ghi không thuộc lớp -> 404, không quản lý lớp (ghi) -> 403. Analytics của
// assignment không tồn tại -> 404. Service giả nhúng interface nil (gọi nhầm method là panic). Bỏ truyền
// actor hoặc bỏ ánh xạ ở attendanceFail / requireAnalyticsAuthErr thì test ĐỎ.

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type s4AttendanceSvc struct {
	service.AttendanceServiceInterface
	err      error
	gotActor uuid.UUID
	gotClass uuid.UUID
}

func (s *s4AttendanceSvc) rec(class, actor uuid.UUID) { s.gotClass, s.gotActor = class, actor }

func (s *s4AttendanceSvc) MarkAttendance(_ context.Context, class, actor uuid.UUID, _ bool, _ dto.BulkCreateAttendanceDTO) ([]dto.AttendanceResponseDTO, error) {
	s.rec(class, actor)
	return nil, s.err
}
func (s *s4AttendanceSvc) GetAllAttendances(_ context.Context, class, actor uuid.UUID, _ bool, _ string, _, _ int) (*dto.AttendanceListResponseDTO, error) {
	s.rec(class, actor)
	return &dto.AttendanceListResponseDTO{}, s.err
}
func (s *s4AttendanceSvc) GetAttendanceByID(_ context.Context, class, _, actor uuid.UUID, _ bool) (*dto.AttendanceResponseDTO, error) {
	s.rec(class, actor)
	return &dto.AttendanceResponseDTO{}, s.err
}
func (s *s4AttendanceSvc) UpdateAttendance(_ context.Context, class, _, actor uuid.UUID, _ bool, _ dto.UpdateAttendanceDTO) (*dto.AttendanceResponseDTO, error) {
	s.rec(class, actor)
	return &dto.AttendanceResponseDTO{}, s.err
}
func (s *s4AttendanceSvc) DeleteAttendance(_ context.Context, class, _, actor uuid.UUID, _ bool) error {
	s.rec(class, actor)
	return s.err
}

func TestS4_AttendanceHandler_MaLoiUyQuyenVaTruyenNguoiGoi(t *testing.T) {
	actor := uuid.New()
	class, id := uuid.NewString(), uuid.NewString()
	base := "/classes/" + class + "/attendances"
	routes := []struct{ method, path, body string }{
		{"POST", base, `{"date":"2026-09-29","attendances":[]}`},
		{"GET", base, ``},
		{"GET", base + "/" + id, ``},
		{"PUT", base + "/" + id, `{"status":"absent"}`},
		{"DELETE", base + "/" + id, ``},
	}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"lớp không xem được", service.ErrClassNotFound, fiber.StatusNotFound},
		{"bản ghi không thuộc lớp", service.ErrAttendanceNotFound, fiber.StatusNotFound},
		{"không quản lý lớp", service.ErrNotClassTeacher, fiber.StatusForbidden},
	} {
		for _, r := range routes {
			t.Run(tc.name+" "+r.method+" "+r.path[len(base):], func(t *testing.T) {
				svc := &s4AttendanceSvc{err: tc.err}
				app := fiber.New()
				app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", actor); return c.Next() })
				h := NewAttendanceHandler(svc, nil)
				g := app.Group("/classes/:classId/attendances")
				g.Post("/", h.MarkAttendance)
				g.Get("/", h.GetAllAttendances)
				g.Get("/:id", h.GetAttendanceByID)
				g.Put("/:id", h.UpdateAttendance)
				g.Delete("/:id", h.DeleteAttendance)
				req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
				req.Header.Set("Content-Type", "application/json")
				res, err := app.Test(req, -1)
				if err != nil {
					t.Fatal(err)
				}
				if res.StatusCode != tc.want {
					t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
				}
				if svc.gotActor != actor || svc.gotClass.String() != class {
					t.Fatalf("service nhận actor=%v class=%v, muốn %v và %v", svc.gotActor, svc.gotClass, actor, class)
				}
			})
		}
	}
}

type s4AnalyticsSvc struct {
	service.AnalyticsServiceInterface
	err error
}

func (s *s4AnalyticsSvc) GetAssignmentAnalytics(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.AssignmentAnalyticsDTO, error) {
	return nil, s.err
}

func TestS4_AnalyticsHandler_AssignmentKhongTonTai404(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"assignment không tồn tại", service.ErrAssignmentNotFound, fiber.StatusNotFound},
		{"không phải chủ", service.ErrNotAnalyticsOwner, fiber.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
			app.Get("/analytics/assignment/:assignmentId", NewAnalyticsHandler(&s4AnalyticsSvc{err: tc.err}, nil).GetAssignmentAnalytics)
			res, err := app.Test(httptest.NewRequest("GET", "/analytics/assignment/"+uuid.NewString(), nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.want {
				t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
			}
		})
	}
}
