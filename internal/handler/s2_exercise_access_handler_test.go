package handler

// Lane S2, lỗi 1 (bài tập khoá học): học viên chỉ nhận test mẫu, không sửa/xoá được bài tập hay
// test case, không xem được bài nộp của người khác. Service giả nhúng interface nil để gọi nhầm
// method là panic. Bỏ requireExerciseManager / canManageExercise ở handler thì test ĐỎ.

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

type s2ExerciseSvc struct {
	service.ExerciseServiceInterface
	canManage        bool
	gotIncludeHidden []bool
	mutated          bool
}

func (s *s2ExerciseSvc) CanManage(context.Context, uuid.UUID, uuid.UUID, bool) (bool, error) {
	return s.canManage, nil
}
func (s *s2ExerciseSvc) GetExerciseByID(_ context.Context, _ uuid.UUID, includeHidden bool) (*dto.ExerciseDetailDTO, error) {
	s.gotIncludeHidden = append(s.gotIncludeHidden, includeHidden)
	return &dto.ExerciseDetailDTO{}, nil
}
func (s *s2ExerciseSvc) GetTestCases(_ context.Context, _ uuid.UUID, includeHidden bool) ([]dto.ExerciseTestCaseResponseDTO, error) {
	s.gotIncludeHidden = append(s.gotIncludeHidden, includeHidden)
	return nil, nil
}
func (s *s2ExerciseSvc) UpdateExercise(context.Context, uuid.UUID, dto.UpdateExerciseDTO) (*dto.ExerciseResponseDTO, error) {
	s.mutated = true
	return &dto.ExerciseResponseDTO{}, nil
}
func (s *s2ExerciseSvc) DeleteExercise(context.Context, uuid.UUID) error {
	s.mutated = true
	return nil
}
func (s *s2ExerciseSvc) DeleteTestCase(context.Context, uuid.UUID, uuid.UUID) error {
	s.mutated = true
	return nil
}
func (s *s2ExerciseSvc) GetSubmissions(context.Context, uuid.UUID, int, int) (*dto.ExerciseSubmissionListDTO, error) {
	s.mutated = true
	return &dto.ExerciseSubmissionListDTO{}, nil
}
func (s *s2ExerciseSvc) CreateExercise(context.Context, uuid.UUID, dto.CreateExerciseDTO) (*dto.ExerciseResponseDTO, error) {
	s.mutated = true
	return &dto.ExerciseResponseDTO{}, nil
}

func s2ExerciseApp(svc *s2ExerciseSvc) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	h := NewExerciseHandler(svc, nil)
	app.Post("/exercises", h.CreateExercise)
	app.Get("/exercises/:id", h.GetExerciseByID)
	app.Put("/exercises/:id", h.UpdateExercise)
	app.Delete("/exercises/:id", h.DeleteExercise)
	app.Get("/exercises/:id/testcases", h.GetTestCases)
	app.Delete("/exercises/:id/testcases/:testCaseId", h.DeleteTestCase)
	app.Get("/exercises/:id/submissions", h.GetSubmissions)
	return app
}

func TestS2_ExerciseHandler_TestCaseAnChiChoNguoiQuanLy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		canManage bool
	}{{"học viên", false}, {"người quản lý", true}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &s2ExerciseSvc{canManage: tc.canManage}
			app := s2ExerciseApp(svc)
			id := uuid.NewString()
			for _, path := range []string{"/exercises/" + id, "/exercises/" + id + "/testcases"} {
				if res, err := app.Test(httptest.NewRequest("GET", path, nil), -1); err != nil || res.StatusCode != fiber.StatusOK {
					t.Fatalf("%s: %v %v", path, res, err)
				}
			}
			for _, got := range svc.gotIncludeHidden {
				if got != tc.canManage {
					t.Fatalf("includeHidden=%v truyền xuống service, muốn %v", got, tc.canManage)
				}
			}
			if len(svc.gotIncludeHidden) != 2 {
				t.Fatalf("service phải được gọi 2 lần, nhận %v", svc.gotIncludeHidden)
			}
		})
	}
}

func TestS2_ExerciseHandler_GhiVaXemBaiNopChiChoNguoiQuanLy(t *testing.T) {
	svc := &s2ExerciseSvc{canManage: false}
	app := s2ExerciseApp(svc)
	id := uuid.NewString()
	for _, r := range []struct{ method, path, body string }{
		{"PUT", "/exercises/" + id, `{"title":"x"}`},
		{"DELETE", "/exercises/" + id, ""},
		{"DELETE", "/exercises/" + id + "/testcases/" + uuid.NewString(), ""},
		{"GET", "/exercises/" + id + "/submissions", ""},
		{"POST", "/exercises", `{"title":"x","description":"d","language":["python"]}`},
	} {
		req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
		req.Header.Set("Content-Type", "application/json")
		if res, err := app.Test(req, -1); err != nil || res.StatusCode != fiber.StatusForbidden {
			t.Errorf("%s %s bởi người không có quyền: %v %v, muốn 403", r.method, r.path, res, err)
		}
	}
	if svc.mutated {
		t.Fatal("service ghi/đọc bài nộp vẫn bị gọi dù người gọi không có quyền")
	}
}
