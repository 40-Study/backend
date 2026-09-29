package handler

// Lane S4, lỗi O-1: gán/gỡ giảng viên và tạo lớp trả đúng mã khi service báo thiếu quyền: lớp không
// xem được -> 404, xem được nhưng không phải chủ -> 403, tạo lớp vào khoá của người khác -> 403.
// Service giả nhúng interface nil (gọi nhầm method là panic). Bỏ ánh xạ trong classErrorStatus hoặc
// bỏ truyền actor ở handler thì test ĐỎ.

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

type s4ClassSvc struct {
	service.ClassServiceInterface
	err       error
	gotActor  uuid.UUID
	gotIsAdmn bool
}

func (s *s4ClassSvc) AssignTeacherToClass(_ context.Context, _, actor uuid.UUID, isAdmin bool, _ dto.AssignTeacherDTO) (*dto.TeacherClassResponseDTO, error) {
	s.gotActor, s.gotIsAdmn = actor, isAdmin
	return &dto.TeacherClassResponseDTO{}, s.err
}
func (s *s4ClassSvc) RemoveTeacherFromClass(_ context.Context, _, _, actor uuid.UUID, isAdmin bool) error {
	s.gotActor, s.gotIsAdmn = actor, isAdmin
	return s.err
}
func (s *s4ClassSvc) CreateClass(_ context.Context, actor uuid.UUID, isAdmin bool, _ dto.CreateClassDTO) (*dto.ClassResponseDTO, error) {
	s.gotActor, s.gotIsAdmn = actor, isAdmin
	return &dto.ClassResponseDTO{}, s.err
}

func TestS4_ClassHandler_MaLoiUyQuyen(t *testing.T) {
	actor := uuid.New()
	routes := []struct{ method, path, body string }{
		{"POST", "/classes/" + uuid.NewString() + "/teachers", `{"teacher_id":"` + uuid.NewString() + `"}`},
		{"DELETE", "/classes/" + uuid.NewString() + "/teachers/" + uuid.NewString(), ``},
		{"POST", "/classes", `{"name":"x"}`},
	}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"lớp không xem được", service.ErrClassNotFound, fiber.StatusNotFound},
		{"xem được nhưng không phải chủ", service.ErrNotClassOwner, fiber.StatusForbidden},
		{"tạo lớp vào khoá người khác", service.ErrNotCourseInstructor, fiber.StatusForbidden},
		{"tạo lớp không phải giảng viên", service.ErrNotTeacher, fiber.StatusForbidden},
	} {
		for _, r := range routes {
			t.Run(tc.name+" "+r.method+" "+r.path[:8], func(t *testing.T) {
				svc := &s4ClassSvc{err: tc.err}
				app := fiber.New()
				app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", actor); return c.Next() })
				h := NewClassHandler(svc, nil)
				app.Post("/classes", h.CreateClass)
				app.Post("/classes/:id/teachers", h.AssignTeacherToClass)
				app.Delete("/classes/:id/teachers/:teacherId", h.RemoveTeacherFromClass)
				req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
				req.Header.Set("Content-Type", "application/json")
				res, err := app.Test(req, -1)
				if err != nil {
					t.Fatal(err)
				}
				// Lỗi của nhóm "tạo lớp" chỉ có nghĩa ở POST /classes, lỗi lớp ở hai route giảng viên;
				// ánh xạ nằm chung ở classErrorStatus nên route nào cũng phải ra đúng mã.
				if res.StatusCode != tc.want {
					t.Fatalf("status=%d, muốn %d", res.StatusCode, tc.want)
				}
				if svc.gotActor != actor {
					t.Fatalf("handler không truyền người gọi xuống service: %v", svc.gotActor)
				}
			})
		}
	}
}
