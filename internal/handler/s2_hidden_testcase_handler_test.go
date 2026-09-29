package handler

// Lane S2, lỗi 1: ?include_hidden=true do CLIENT tự bật, nên handler chỉ được truyền includeHidden
// =true xuống service khi người gọi là chủ assignment/admin. Test chạy qua HTTP thật (fiber) với
// service giả ghi lại tham số. Bỏ `&& h.canSeeHiddenTests(...)` thì các test này ĐỎ.

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type s2AssignmentSvc struct {
	// nhúng interface nil: gọi nhầm method chưa override sẽ panic, lộ ngay việc handler đi đường lạ
	service.AssignmentServiceInterface
	canManage         bool
	gotIncludeHidden  []bool
	deleteAssignment  uuid.UUID
	addTestCaseCalled bool
}

func (s *s2AssignmentSvc) CanManage(context.Context, uuid.UUID, uuid.UUID, bool) (bool, error) {
	return s.canManage, nil
}
func (s *s2AssignmentSvc) GetByID(_ context.Context, id uuid.UUID, includeHidden bool) (*model.Assignment, error) {
	s.gotIncludeHidden = append(s.gotIncludeHidden, includeHidden)
	return &model.Assignment{BaseModel: model.BaseModel{ID: id}}, nil
}
func (s *s2AssignmentSvc) GetTestCases(_ context.Context, _ uuid.UUID, includeHidden bool) ([]model.TestCase, error) {
	s.gotIncludeHidden = append(s.gotIncludeHidden, includeHidden)
	return nil, nil
}
func (s *s2AssignmentSvc) AddTestCase(context.Context, uuid.UUID, dto.CreateTestCaseDTO) (*model.TestCase, error) {
	s.addTestCaseCalled = true
	return &model.TestCase{}, nil
}
func (s *s2AssignmentSvc) DeleteTestCase(_ context.Context, assignmentID, _ uuid.UUID) error {
	s.deleteAssignment = assignmentID
	return nil
}

func s2AssignmentApp(svc *s2AssignmentSvc) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	h := NewAssignmentHandler(svc, nil, nil)
	app.Get("/assignments/:id", h.GetByID)
	app.Get("/assignments/:id/testcases", h.GetTestCases)
	app.Post("/assignments/:id/testcases", h.AddTestCase)
	app.Delete("/assignments/:id/testcases/:tcId", h.DeleteTestCase)
	return app
}

func TestS2_AssignmentHandler_IncludeHiddenChiChoChuAssignment(t *testing.T) {
	for _, tc := range []struct {
		name      string
		canManage bool
		want      bool
	}{{"người không phải chủ tự bật include_hidden", false, false}, {"chủ assignment bật include_hidden", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &s2AssignmentSvc{canManage: tc.canManage}
			app := s2AssignmentApp(svc)
			id := uuid.NewString()
			for _, path := range []string{"/assignments/" + id + "?include_hidden=true", "/assignments/" + id + "/testcases?include_hidden=true"} {
				res, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
				if err != nil || res.StatusCode != fiber.StatusOK {
					t.Fatalf("%s: status=%v err=%v", path, res, err)
				}
			}
			if len(svc.gotIncludeHidden) != 2 {
				t.Fatalf("service phải được gọi 2 lần, nhận %v", svc.gotIncludeHidden)
			}
			for _, got := range svc.gotIncludeHidden {
				if got != tc.want {
					t.Fatalf("includeHidden truyền xuống service = %v, muốn %v", got, tc.want)
				}
			}
		})
	}
}

func TestS2_AssignmentHandler_GhiTestCaseChiChoChuAssignment(t *testing.T) {
	svc := &s2AssignmentSvc{canManage: false}
	app := s2AssignmentApp(svc)
	id := uuid.NewString()

	post := httptest.NewRequest("POST", "/assignments/"+id+"/testcases", nil)
	post.Header.Set("Content-Type", "application/json")
	if res, err := app.Test(post, -1); err != nil || res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("thêm test case bởi người lạ: %v %v, muốn 403", res, err)
	}
	del := httptest.NewRequest("DELETE", "/assignments/"+id+"/testcases/"+uuid.NewString(), nil)
	if res, err := app.Test(del, -1); err != nil || res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("xoá test case bởi người lạ: %v %v, muốn 403", res, err)
	}
	if svc.addTestCaseCalled || svc.deleteAssignment != uuid.Nil {
		t.Fatal("service ghi test case vẫn bị gọi dù người gọi không có quyền")
	}
}

// Chủ assignment xoá test case: service phải nhận assignmentID của route để xoá có phạm vi, không
// xoá chéo test case của assignment khác.
func TestS2_AssignmentHandler_XoaTestCaseTruyenAssignmentID(t *testing.T) {
	svc := &s2AssignmentSvc{canManage: true}
	app := s2AssignmentApp(svc)
	id := uuid.New()
	del := httptest.NewRequest("DELETE", "/assignments/"+id.String()+"/testcases/"+uuid.NewString(), nil)
	if res, err := app.Test(del, -1); err != nil || res.StatusCode != fiber.StatusOK {
		t.Fatalf("chủ xoá test case: %v %v, muốn 200", res, err)
	}
	if svc.deleteAssignment != id {
		t.Fatalf("service nhận assignmentID %v, muốn %v", svc.deleteAssignment, id)
	}
}
