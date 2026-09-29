package handler

// Lane S3, lỗi 1: route GHI của assignment (tạo/sửa/xoá/publish/unpublish/test case) chỉ chủ hoặc
// admin được dùng; route ĐỌC (đề, sandbox, test mẫu, danh sách theo phiên) chỉ người xem được.
// Quy ước: không xem được -> 404 (không lộ id có tồn tại), xem được nhưng không phải chủ -> 403.
// Test chạy qua HTTP thật (fiber) với service giả nhúng interface nil (gọi nhầm method là panic) và
// ghi lại việc "có bị ghi/đọc hay không". Bỏ requireAssignmentManager/requireAssignmentViewer ở
// handler thì các test này ĐỎ.

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type s3AssignmentSvc struct {
	service.AssignmentServiceInterface
	canManage, canView bool
	createErr          error
	mutated, read      bool
}

func (s *s3AssignmentSvc) CanManage(context.Context, uuid.UUID, uuid.UUID, bool) (bool, error) {
	return s.canManage, nil
}
func (s *s3AssignmentSvc) CanView(context.Context, uuid.UUID, uuid.UUID, bool) (bool, error) {
	return s.canManage || s.canView, nil
}
func (s *s3AssignmentSvc) Create(context.Context, uuid.UUID, bool, dto.CreateAssignmentDTO) (*model.Assignment, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.mutated = true
	return &model.Assignment{}, nil
}
func (s *s3AssignmentSvc) Update(context.Context, uuid.UUID, dto.UpdateAssignmentDTO) (*model.Assignment, error) {
	s.mutated = true
	return &model.Assignment{}, nil
}
func (s *s3AssignmentSvc) Delete(context.Context, uuid.UUID) error { s.mutated = true; return nil }
func (s *s3AssignmentSvc) Publish(context.Context, uuid.UUID, service.LivekitServiceInterface) (*model.Assignment, error) {
	s.mutated = true
	return &model.Assignment{}, nil
}
func (s *s3AssignmentSvc) Unpublish(context.Context, uuid.UUID) (*model.Assignment, error) {
	s.mutated = true
	return &model.Assignment{}, nil
}
func (s *s3AssignmentSvc) GetByID(context.Context, uuid.UUID, bool) (*model.Assignment, error) {
	s.read = true
	return &model.Assignment{StarterCode: "secret-starter"}, nil
}
func (s *s3AssignmentSvc) GetTestCases(context.Context, uuid.UUID, bool) ([]model.TestCase, error) {
	s.read = true
	return nil, nil
}
func (s *s3AssignmentSvc) GetSandbox(context.Context, uuid.UUID, uuid.UUID) (*dto.SandboxResponseDTO, error) {
	s.read = true
	return &dto.SandboxResponseDTO{}, nil
}
func (s *s3AssignmentSvc) GetBySession(context.Context, uuid.UUID, bool, uuid.UUID, int, int) (*dto.AssignmentListDTO, error) {
	s.read = true
	if s.createErr != nil { // tái dùng trường làm "lỗi trả về" cho case người ngoài phiên
		return nil, s.createErr
	}
	return &dto.AssignmentListDTO{}, nil
}
func (s *s3AssignmentSvc) AddTestCase(context.Context, uuid.UUID, dto.CreateTestCaseDTO) (*model.TestCase, error) {
	s.mutated = true
	return &model.TestCase{}, nil
}
func (s *s3AssignmentSvc) ImportTestCases(context.Context, uuid.UUID, dto.ImportTestCasesDTO) ([]model.TestCase, error) {
	s.mutated = true
	return nil, nil
}
func (s *s3AssignmentSvc) DeleteTestCase(context.Context, uuid.UUID, uuid.UUID) error {
	s.mutated = true
	return nil
}

func s3AssignmentApp(svc *s3AssignmentSvc) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	h := NewAssignmentHandler(svc, nil, nil)
	app.Post("/assignments", h.Create)
	app.Get("/assignments", h.GetBySession)
	app.Get("/assignments/:id", h.GetByID)
	app.Get("/assignments/:id/sandbox", h.GetSandbox)
	app.Put("/assignments/:id", h.Update)
	app.Delete("/assignments/:id", h.Delete)
	app.Post("/assignments/:id/publish", h.Publish)
	app.Post("/assignments/:id/unpublish", h.Unpublish)
	app.Get("/assignments/:id/testcases", h.GetTestCases)
	app.Post("/assignments/:id/testcases", h.AddTestCase)
	app.Post("/assignments/:id/testcases/import", h.ImportTestCases)
	app.Delete("/assignments/:id/testcases/:tcId", h.DeleteTestCase)
	return app
}

func s3Do(t *testing.T, app *fiber.App, method, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

// Route ghi: người lạ (không xem được) 404, học viên xem được 403, chủ 200 — và chỉ chủ mới làm
// service bị ghi.
func TestS3_AssignmentHandler_RouteGhiChiChoChu(t *testing.T) {
	id := uuid.NewString()
	writes := []struct{ method, path string }{
		{"PUT", "/assignments/" + id},
		{"DELETE", "/assignments/" + id},
		{"POST", "/assignments/" + id + "/publish"},
		{"POST", "/assignments/" + id + "/unpublish"},
		{"POST", "/assignments/" + id + "/testcases"},
		{"POST", "/assignments/" + id + "/testcases/import"},
		{"DELETE", "/assignments/" + id + "/testcases/" + uuid.NewString()},
	}
	roles := []struct {
		name               string
		canManage, canView bool
		want               int
		wantMutated        bool
	}{
		{"người lạ, không xem được", false, false, fiber.StatusNotFound, false},
		{"học viên xem được nhưng không phải chủ", false, true, fiber.StatusForbidden, false},
		{"chủ (giảng viên chủ khoá/admin)", true, true, fiber.StatusOK, true},
	}
	for _, w := range writes {
		for _, r := range roles {
			t.Run(w.method+" "+w.path[len("/assignments/"+id):]+" / "+r.name, func(t *testing.T) {
				svc := &s3AssignmentSvc{canManage: r.canManage, canView: r.canView}
				status, _ := s3Do(t, s3AssignmentApp(svc), w.method, w.path)
				// import/thêm test case của chủ trả 201, phần còn lại 200.
				if r.wantMutated && status != fiber.StatusOK && status != fiber.StatusCreated {
					t.Fatalf("status=%d, muốn 200/201", status)
				}
				if !r.wantMutated && status != r.want {
					t.Fatalf("status=%d, muốn %d", status, r.want)
				}
				if svc.mutated != r.wantMutated {
					t.Fatalf("service bị ghi=%v, muốn %v", svc.mutated, r.wantMutated)
				}
			})
		}
	}
}

// Route đọc: người không xem được nhận 404 và service không bị đọc; người xem được nhận 200.
func TestS3_AssignmentHandler_RouteDocChiChoNguoiXemDuoc(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{"/assignments/" + id, "/assignments/" + id + "/sandbox", "/assignments/" + id + "/testcases"} {
		t.Run(path[len("/assignments/"+id):]+"/người lạ", func(t *testing.T) {
			svc := &s3AssignmentSvc{}
			status, body := s3Do(t, s3AssignmentApp(svc), "GET", path)
			if status != fiber.StatusNotFound {
				t.Fatalf("người lạ đọc %s: status=%d, muốn 404", path, status)
			}
			if svc.read || strings.Contains(body, "secret-starter") {
				t.Fatalf("service vẫn bị đọc/lộ nội dung cho người lạ: read=%v body=%s", svc.read, body)
			}
		})
		t.Run(path[len("/assignments/"+id):]+"/học viên trong lớp", func(t *testing.T) {
			svc := &s3AssignmentSvc{canView: true}
			if status, _ := s3Do(t, s3AssignmentApp(svc), "GET", path); status != fiber.StatusOK {
				t.Fatalf("học viên đọc %s: status=%d, muốn 200", path, status)
			}
		})
	}
}

// Danh sách theo phiên: service báo "không thuộc phiên" (ErrAssignmentNotFound) -> 404.
func TestS3_AssignmentHandler_DanhSachTheoPhienNguoiNgoai404(t *testing.T) {
	svc := &s3AssignmentSvc{createErr: service.ErrAssignmentNotFound}
	status, _ := s3Do(t, s3AssignmentApp(svc), "GET", "/assignments?session_id="+uuid.NewString())
	if status != fiber.StatusNotFound {
		t.Fatalf("status=%d, muốn 404", status)
	}
}

// Tạo: lỗi uỷ quyền của service ánh xạ đúng mã; không có lỗi thì 201.
func TestS3_AssignmentHandler_TaoAnhXaLoiUyQuyen(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"không phải chủ phiên/lớp", service.ErrAssignmentForbidden, fiber.StatusForbidden},
		{"không gắn vào đâu", service.ErrAssignmentTargetRequired, fiber.StatusBadRequest},
		{"chủ phiên/lớp", nil, fiber.StatusCreated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &s3AssignmentSvc{createErr: tc.err}
			if status, _ := s3Do(t, s3AssignmentApp(svc), "POST", "/assignments"); status != tc.want {
				t.Fatalf("status=%d, muốn %d", status, tc.want)
			}
		})
	}
}
