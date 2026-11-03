package handler

// GET /me/timetable: ?organization_id / ?scope=personal được đọc thành bộ lọc nguồn lớp và truyền xuống service
// ở cả hai nhánh (có/không khoảng ngày); id sai, scope lạ hoặc gửi cả hai -> 400, không gọi service.

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type orgFilterTimetableSvc struct {
	service.ScheduleServiceInterface
	called bool
	filter dto.TimetableOrgFilter
}

func (s *orgFilterTimetableSvc) GetMyTimetable(_ context.Context, _ uuid.UUID, _ string, f dto.TimetableOrgFilter) (*dto.TimetableResponseDTO, error) {
	s.called, s.filter = true, f
	return &dto.TimetableResponseDTO{}, nil
}

func (s *orgFilterTimetableSvc) GetMyTimetableWithSessions(_ context.Context, _ uuid.UUID, _ string, _, _ time.Time, f dto.TimetableOrgFilter) (*dto.TimetableResponseDTO, error) {
	s.called, s.filter = true, f
	return &dto.TimetableResponseDTO{}, nil
}

func TestTimetableOrgFilter_Handler(t *testing.T) {
	orgID := uuid.New()
	const rng = "&sessions_from=2031-03-01&sessions_to=2031-03-31"
	for _, tc := range []struct {
		name  string
		query string
		want  int
		check func(dto.TimetableOrgFilter) bool
	}{
		{"không lọc", "?", fiber.StatusOK, func(f dto.TimetableOrgFilter) bool { return f.OrganizationID == nil && !f.PersonalOnly }},
		{"lớp cá nhân", "?scope=personal", fiber.StatusOK, func(f dto.TimetableOrgFilter) bool { return f.PersonalOnly && f.OrganizationID == nil }},
		{"lớp cá nhân kèm buổi", "?scope=personal" + rng, fiber.StatusOK, func(f dto.TimetableOrgFilter) bool { return f.PersonalOnly }},
		{"theo tổ chức", "?organization_id=" + orgID.String(), fiber.StatusOK, func(f dto.TimetableOrgFilter) bool { return f.OrganizationID != nil && *f.OrganizationID == orgID }},
		{"theo tổ chức kèm buổi", "?organization_id=" + orgID.String() + rng, fiber.StatusOK, func(f dto.TimetableOrgFilter) bool { return f.OrganizationID != nil && *f.OrganizationID == orgID }},
		{"id sai", "?organization_id=abc", fiber.StatusBadRequest, nil},
		{"scope lạ", "?scope=org", fiber.StatusBadRequest, nil},
		{"gửi cả hai", "?scope=personal&organization_id=" + orgID.String(), fiber.StatusBadRequest, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &orgFilterTimetableSvc{}
			app := mountWithCaller("GET", "/timetable/me", uuid.New(), NewScheduleHandler(svc, nil).GetMyTimetable)
			res, err := app.Test(httptest.NewRequest("GET", "/timetable/me"+tc.query, nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.want {
				t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
			}
			if tc.check == nil {
				if svc.called {
					t.Fatal("400 nhưng vẫn gọi service")
				}
				return
			}
			if !svc.called || !tc.check(svc.filter) {
				t.Fatalf("bộ lọc truyền xuống sai: called=%v filter=%+v", svc.called, svc.filter)
			}
		})
	}
}
