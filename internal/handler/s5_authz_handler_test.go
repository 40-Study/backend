package handler

// Lane S5: các route lịch học/buổi học của lớp truyền người gọi xuống service và ánh xạ lỗi uỷ quyền
// (404 lớp/lịch/buổi không xem được hoặc không tồn tại, 403 xem được nhưng không quản lý). Danh sách ghi danh
// của khoá: 404 khoá ẩn, 403 không phải chủ khoá. Báo cáo: 404 cho người không liên quan. Service giả nhúng
// interface nil (gọi nhầm method là panic). Bỏ truyền actor hoặc bỏ ánh xạ thì test ĐỎ.

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

type s5ScheduleSvc struct {
	service.ScheduleServiceInterface
	err      error
	gotActor uuid.UUID
}

func (s *s5ScheduleSvc) rec(actor uuid.UUID) { s.gotActor = actor }

func (s *s5ScheduleSvc) CreateSchedule(_ context.Context, _, actor uuid.UUID, _ bool, _ dto.CreateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error) {
	s.rec(actor)
	return &dto.ClassScheduleResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) GetSchedulesByClass(_ context.Context, _, actor uuid.UUID, _ bool) ([]dto.ClassScheduleResponseDTO, error) {
	s.rec(actor)
	return nil, s.err
}
func (s *s5ScheduleSvc) GetScheduleByID(_ context.Context, _, actor uuid.UUID, _ bool) (*dto.ClassScheduleResponseDTO, error) {
	s.rec(actor)
	return &dto.ClassScheduleResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) UpdateSchedule(_ context.Context, _, actor uuid.UUID, _ bool, _ dto.UpdateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error) {
	s.rec(actor)
	return &dto.ClassScheduleResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) DeleteSchedule(_ context.Context, _, actor uuid.UUID, _ bool) error {
	s.rec(actor)
	return s.err
}
func (s *s5ScheduleSvc) GetClassTimetable(_ context.Context, _, actor uuid.UUID, _ bool) (*dto.TimetableResponseDTO, error) {
	s.rec(actor)
	return &dto.TimetableResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) CreateSession(_ context.Context, _, actor uuid.UUID, _ bool, _ dto.CreateClassSessionDTO) (*dto.ClassSessionResponseDTO, error) {
	s.rec(actor)
	return &dto.ClassSessionResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) GetSessionsByClass(_ context.Context, _, actor uuid.UUID, _ bool, _, _ int) (*dto.ClassSessionListDTO, error) {
	s.rec(actor)
	return &dto.ClassSessionListDTO{}, s.err
}
func (s *s5ScheduleSvc) GetSessionByID(_ context.Context, _, actor uuid.UUID, _ bool) (*dto.ClassSessionResponseDTO, error) {
	s.rec(actor)
	return &dto.ClassSessionResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) UpdateSession(_ context.Context, _, actor uuid.UUID, _ bool, _ dto.UpdateClassSessionDTO) (*dto.ClassSessionResponseDTO, error) {
	s.rec(actor)
	return &dto.ClassSessionResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) CancelSession(_ context.Context, _, actor uuid.UUID, _ bool, _ string) error {
	s.rec(actor)
	return s.err
}
func (s *s5ScheduleSvc) GenerateSessions(_ context.Context, _, actor uuid.UUID, _ bool, _ dto.GenerateSessionsDTO) ([]dto.ClassSessionResponseDTO, error) {
	s.rec(actor)
	return nil, s.err
}
func (s *s5ScheduleSvc) GetSessionAttendances(_ context.Context, _, actor uuid.UUID, _ bool) ([]dto.SessionAttendanceResponseDTO, error) {
	s.rec(actor)
	return nil, s.err
}
func (s *s5ScheduleSvc) MarkAttendance(_ context.Context, _ uuid.UUID, _ dto.MarkAttendanceDTO, actor uuid.UUID, _ bool) (*dto.SessionAttendanceResponseDTO, error) {
	s.rec(actor)
	return &dto.SessionAttendanceResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) BulkMarkAttendance(_ context.Context, _ uuid.UUID, _ dto.BulkMarkAttendanceDTO, actor uuid.UUID, _ bool) ([]dto.SessionAttendanceResponseDTO, error) {
	s.rec(actor)
	return nil, s.err
}
func (s *s5ScheduleSvc) UpdateAttendance(_ context.Context, _, _ uuid.UUID, _ dto.UpdateSessionAttendanceDTO, actor uuid.UUID, _ bool) (*dto.SessionAttendanceResponseDTO, error) {
	s.rec(actor)
	return &dto.SessionAttendanceResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) StudentCheckIn(_ context.Context, _, actor uuid.UUID) (*dto.SessionAttendanceResponseDTO, error) {
	s.rec(actor)
	return &dto.SessionAttendanceResponseDTO{}, s.err
}
func (s *s5ScheduleSvc) StudentCheckOut(_ context.Context, _, actor uuid.UUID) (*dto.SessionAttendanceResponseDTO, error) {
	s.rec(actor)
	return &dto.SessionAttendanceResponseDTO{}, s.err
}

func TestS5_ScheduleHandler_MaLoiUyQuyenVaTruyenNguoiGoi(t *testing.T) {
	actor := uuid.New()
	class, id, session := uuid.NewString(), uuid.NewString(), uuid.NewString()
	const day = `"08:00"`
	routes := []struct{ method, path, body string }{
		{"POST", "/classes/" + class + "/schedules/", `{"day_of_week":1,"start_time":` + day + `,"end_time":` + day + `,"effective_from":"2026-01-01"}`},
		{"GET", "/classes/" + class + "/schedules/", ``},
		{"GET", "/classes/" + class + "/schedules/" + id, ``},
		{"PUT", "/classes/" + class + "/schedules/" + id, `{"room":"a"}`},
		{"DELETE", "/classes/" + class + "/schedules/" + id, ``},
		{"GET", "/classes/" + class + "/timetable", ``},
		{"POST", "/classes/" + class + "/sessions/", `{"date":"2026-01-01","start_time":` + day + `,"end_time":` + day + `}`},
		{"GET", "/classes/" + class + "/sessions/", ``},
		{"GET", "/classes/" + class + "/sessions/" + id, ``},
		{"PUT", "/classes/" + class + "/sessions/" + id, `{"topic":"a"}`},
		{"DELETE", "/classes/" + class + "/sessions/" + id, `{"cancel_reason":"x"}`},
		{"POST", "/classes/" + class + "/sessions/generate", `{"start_date":"2026-01-01","end_date":"2026-01-07"}`},
		{"GET", "/sessions/" + session + "/attendances/", ``},
		{"POST", "/sessions/" + session + "/attendances/", `{"student_id":"` + uuid.NewString() + `","status":"present"}`},
		{"POST", "/sessions/" + session + "/attendances/bulk", `{"attendances":[{"student_id":"` + uuid.NewString() + `","status":"present"}]}`},
		{"PUT", "/sessions/" + session + "/attendances/" + id, `{"status":"absent"}`},
		{"POST", "/sessions/" + session + "/check-in", ``},
		{"POST", "/sessions/" + session + "/check-out", ``},
	}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"lớp không xem được", service.ErrClassNotFound, fiber.StatusNotFound},
		{"lịch không tồn tại", service.ErrScheduleNotFound, fiber.StatusNotFound},
		{"buổi không tồn tại", service.ErrClassSessionNotFound, fiber.StatusNotFound},
		{"không quản lý lớp", service.ErrNotClassTeacher, fiber.StatusForbidden},
	} {
		for _, r := range routes {
			t.Run(tc.name+" "+r.method+" "+r.path, func(t *testing.T) {
				svc := &s5ScheduleSvc{err: tc.err}
				app := fiber.New()
				app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", actor); return c.Next() })
				h := NewScheduleHandler(svc, nil)
				cs := app.Group("/classes/:classId/schedules")
				cs.Post("/", h.CreateSchedule)
				cs.Get("/", h.GetSchedulesByClass)
				cs.Get("/:id", h.GetScheduleByID)
				cs.Put("/:id", h.UpdateSchedule)
				cs.Delete("/:id", h.DeleteSchedule)
				app.Get("/classes/:classId/timetable", h.GetClassTimetable)
				se := app.Group("/classes/:classId/sessions")
				se.Get("/", h.GetSessionsByClass)
				se.Post("/", h.CreateSession)
				se.Get("/:id", h.GetSessionByID)
				se.Put("/:id", h.UpdateSession)
				se.Delete("/:id", h.CancelSession)
				se.Post("/generate", h.GenerateSessions)
				sa := app.Group("/sessions/:sessionId/attendances")
				sa.Get("/", h.GetSessionAttendances)
				sa.Post("/", h.MarkAttendance)
				sa.Post("/bulk", h.BulkMarkAttendance)
				sa.Put("/:id", h.UpdateAttendance)
				app.Post("/sessions/:sessionId/check-in", h.StudentCheckIn)
				app.Post("/sessions/:sessionId/check-out", h.StudentCheckOut)

				req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
				req.Header.Set("Content-Type", "application/json")
				res, err := app.Test(req, -1)
				if err != nil {
					t.Fatal(err)
				}
				if res.StatusCode != tc.want {
					t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
				}
				if svc.gotActor != actor {
					t.Fatalf("service nhận actor=%v, muốn %v", svc.gotActor, actor)
				}
			})
		}
	}
}

type s5EnrollmentSvc struct {
	service.EnrollmentServiceInterface
	err      error
	gotActor uuid.UUID
}

func (s *s5EnrollmentSvc) GetCourseEnrollments(_ context.Context, _, actor uuid.UUID, _ bool, _, _ int) (*dto.CourseEnrollmentListDTO, error) {
	s.gotActor = actor
	return &dto.CourseEnrollmentListDTO{}, s.err
}

func TestS5_EnrollmentHandler_DanhSachGhiDanhMaLoi(t *testing.T) {
	actor := uuid.New()
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"khoá ẩn", service.ErrCourseHidden, fiber.StatusNotFound},
		{"không phải chủ khoá", service.ErrNotCourseOwner, fiber.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &s5EnrollmentSvc{err: tc.err}
			app := fiber.New()
			app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", actor); return c.Next() })
			app.Get("/courses/:courseId/enrollments", NewEnrollmentHandler(svc, nil).GetCourseEnrollments)
			res, err := app.Test(httptest.NewRequest("GET", "/courses/"+uuid.NewString()+"/enrollments", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.want {
				t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
			}
			if svc.gotActor != actor {
				t.Fatalf("service nhận actor=%v, muốn %v", svc.gotActor, actor)
			}
		})
	}
}