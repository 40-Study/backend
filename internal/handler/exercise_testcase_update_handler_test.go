package handler

// Lane BE-2 (code-editor gap #2): PUT /exercises/:id/testcases/:testCaseId và giới hạn chấm của
// PUT /exercises/:id. Service giả nhúng interface nil — gọi nhầm method là panic.

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type be2TestCaseSvc struct {
	service.ExerciseServiceInterface
	canManage bool
	notFound  bool
	gotTC     *dto.UpdateExerciseTestCaseDTO
	gotEx     *dto.UpdateExerciseDTO
}

func (s *be2TestCaseSvc) CanManage(context.Context, uuid.UUID, uuid.UUID, bool) (bool, error) {
	return s.canManage, nil
}
func (s *be2TestCaseSvc) UpdateTestCase(_ context.Context, exerciseID, id uuid.UUID, req dto.UpdateExerciseTestCaseDTO) (*dto.ExerciseTestCaseResponseDTO, error) {
	s.gotTC = &req
	if s.notFound {
		return nil, service.ErrExerciseTestCaseNotFound
	}
	return &dto.ExerciseTestCaseResponseDTO{ID: id, ExerciseID: exerciseID, Input: "in", ExpectedOutput: "out", IsHidden: true, DisplayOrder: 2}, nil
}
func (s *be2TestCaseSvc) UpdateExercise(_ context.Context, _ uuid.UUID, req dto.UpdateExerciseDTO) (*dto.ExerciseResponseDTO, error) {
	s.gotEx = &req
	return &dto.ExerciseResponseDTO{}, nil
}

func be2Do(t *testing.T, svc *be2TestCaseSvc, method, path, body string) (int, map[string]any) {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	h := NewExerciseHandler(svc, nil)
	app.Put("/exercises/:id", h.UpdateExercise)
	app.Put("/exercises/:id/testcases/:testCaseId", h.UpdateTestCase)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestBE2_UpdateTestCaseHandler(t *testing.T) {
	ex, tc := uuid.NewString(), uuid.NewString()
	path := "/exercises/" + ex + "/testcases/" + tc

	t.Run("không quản lý bài tập -> 403, không sửa", func(t *testing.T) {
		svc := &be2TestCaseSvc{}
		if code, _ := be2Do(t, svc, "PUT", path, `{"input":"x"}`); code != 403 || svc.gotTC != nil {
			t.Fatalf("muốn 403 và không gọi service, nhận %d (gọi=%v)", code, svc.gotTC != nil)
		}
	})

	t.Run("sửa từng phần -> 200 cùng dạng response tạo", func(t *testing.T) {
		svc := &be2TestCaseSvc{canManage: true}
		code, out := be2Do(t, svc, "PUT", path, `{"expected_output":"42","is_hidden":false}`)
		if code != 200 {
			t.Fatalf("muốn 200, nhận %d %v", code, out)
		}
		g := svc.gotTC
		if g.Input != nil || g.DisplayOrder != nil || g.ExpectedOutput == nil || *g.ExpectedOutput != "42" || g.IsHidden == nil || *g.IsHidden {
			t.Fatalf("trường không gửi phải là nil, trường gửi giữ giá trị (kể cả false): %+v", g)
		}
		data, _ := out["data"].(map[string]any)
		for _, k := range []string{"id", "exercise_id", "input", "expected_output", "is_hidden", "display_order"} {
			if _, ok := data[k]; !ok {
				t.Errorf("response.data thiếu %q: %v", k, out)
			}
		}
		if data["id"] != tc || data["exercise_id"] != ex {
			t.Errorf("response.data sai id: %v", data)
		}
	})

	t.Run("test case không thuộc bài tập -> 404", func(t *testing.T) {
		svc := &be2TestCaseSvc{canManage: true, notFound: true}
		if code, out := be2Do(t, svc, "PUT", path, `{"input":"x"}`); code != 404 {
			t.Fatalf("muốn 404, nhận %d %v", code, out)
		}
	})

	t.Run("input rỗng / id sai -> 400", func(t *testing.T) {
		svc := &be2TestCaseSvc{canManage: true}
		if code, _ := be2Do(t, svc, "PUT", path, `{"input":""}`); code != 400 || svc.gotTC != nil {
			t.Fatalf("input rỗng muốn 400, nhận %d", code)
		}
		if code, _ := be2Do(t, svc, "PUT", "/exercises/"+ex+"/testcases/khong-phai-uuid", `{}`); code != 400 {
			t.Fatalf("testCaseId sai muốn 400, nhận %d", code)
		}
	})
}

func TestBE2_UpdateExerciseHandler_ValidatesJudgeLimits(t *testing.T) {
	path := "/exercises/" + uuid.NewString()
	for _, body := range []string{
		`{"time_limit":0}`, `{"time_limit":-1}`, `{"time_limit":21}`,
		`{"memory_limit":8}`, `{"memory_limit":2048}`,
		`{"pass_percentage":0}`, `{"pass_percentage":101}`,
	} {
		svc := &be2TestCaseSvc{canManage: true}
		if code, _ := be2Do(t, svc, "PUT", path, body); code != 400 || svc.gotEx != nil {
			t.Errorf("%s: muốn 400 và không lưu, nhận %d", body, code)
		}
	}
	svc := &be2TestCaseSvc{canManage: true}
	code, _ := be2Do(t, svc, "PUT", path, `{"time_limit":20,"memory_limit":512,"pass_percentage":80}`)
	if code != 200 || svc.gotEx == nil || *svc.gotEx.TimeLimit != 20 || *svc.gotEx.MemoryLimit != 512 || *svc.gotEx.PassPercentage != 80 {
		t.Fatalf("giới hạn hợp lệ muốn 200 và chuyển nguyên giá trị, nhận %d %+v", code, svc.gotEx)
	}
}
