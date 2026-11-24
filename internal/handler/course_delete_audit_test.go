package handler_test

// QA4-A09: admin xoá khoá học phải để lại dòng nhật ký quản trị course.delete; giảng viên tự xoá khoá
// của mình thì KHÔNG (DELETE /courses/:id dùng chung cho cả hai). Handler THẬT + PermissionChecker
// THẬT (repo vai trò giả); chỉ service xoá được thay bằng stub.
// Mutation đã kiểm: bỏ middleware.SkipAudit khỏi DeleteCourse -> nhánh giảng viên ĐỎ.

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type courseDeleteStub struct{ service.CourseServiceInterface }

func (courseDeleteStub) DeleteCourse(context.Context, uuid.UUID, uuid.UUID, bool) error { return nil }

type courseDeleteAuditSpy struct {
	mu      sync.Mutex
	entries []model.AuditEntry
}

func (s *courseDeleteAuditSpy) Record(_ context.Context, e model.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return nil
}

func TestCourseDelete_AuditOnlyForAdmin(t *testing.T) {
	adminID, teacherID := uuid.New(), uuid.New()
	adminRole, teacherRole := uuid.New(), uuid.New()
	sysRoles := &asFakeSystemRoleRepo{perms: map[uuid.UUID][]string{
		adminRole:   {"SYSTEM_SETTINGS_MANAGE"},
		teacherRole: {"COURSES_CREATE", "COURSES_UPDATE_OWN"},
	}}
	userRoles := &asFakeUserSystemRoleRepo{byUser: map[uuid.UUID][]model.UserSystemRole{
		adminID:   {{UserID: adminID, SystemRoleID: adminRole, Status: model.UserSystemRoleStatusActive}},
		teacherID: {{UserID: teacherID, SystemRoleID: teacherRole, Status: model.UserSystemRoleStatusActive}},
	}}
	h := handler.NewCourseHandler(courseDeleteStub{}, middleware.NewPermissionChecker(userRoles, sysRoles, nil, nil))

	for _, tc := range []struct {
		name  string
		actor uuid.UUID
		want  int
	}{{"admin", adminID, 1}, {"giảng viên chủ khoá", teacherID, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &courseDeleteAuditSpy{}
			app := fiber.New()
			app.Delete("/courses/:id", func(c *fiber.Ctx) error { c.Locals("user_id", tc.actor); return c.Next() },
				middleware.Audit(rec, model.AuditActionCourseDelete, "course", "id"), h.DeleteCourse)
			courseID := uuid.NewString()
			resp, err := app.Test(httptest.NewRequest("DELETE", "/courses/"+courseID, nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status %d, muốn 200", resp.StatusCode)
			}
			if len(rec.entries) != tc.want {
				t.Fatalf("%d dòng nhật ký, muốn %d", len(rec.entries), tc.want)
			}
			if tc.want == 1 && (rec.entries[0].TargetID != courseID || rec.entries[0].ActorID != tc.actor) {
				t.Errorf("entry = %+v", rec.entries[0])
			}
		})
	}
}
