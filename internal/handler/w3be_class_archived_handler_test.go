package handler

// Lane W3-BE: hợp đồng HTTP của lớp lưu trữ (ghi vào lớp archived -> 409 + code CLASS_ARCHIVED, message tiếng Việt) ở mọi
// nhóm route ghi, và luật "không xem được thì 404" của livestream/analytics. Đổi một bảng ánh xạ mà bỏ ErrClassArchived
// thì ca tương ứng ĐỎ (rơi về 400/500, không có code).

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type w3Body struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

func w3Do(t *testing.T, handler fiber.Handler, method, body string) (int, w3Body) {
	t.Helper()
	app := fiber.New()
	app.Add(method, "/x/:classId", handler)
	req := httptest.NewRequest(method, "/x/"+uuid.NewString(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var b w3Body
	_ = json.Unmarshal(raw, &b)
	return resp.StatusCode, b
}

func TestW3BE_ClassArchived_Map409VaCode_MoiNhomRouteGhi(t *testing.T) {
	archived := service.ErrClassArchived
	if archived.Error() == "" {
		t.Fatal("ErrClassArchived phải có message")
	}
	cases := map[string]fiber.Handler{
		"lớp (ghi danh/gán GV)": func(c *fiber.Ctx) error {
			if status := classErrorStatus(archived); status != 0 {
				return c.Status(status).JSON(classErrorBody(archived))
			}
			return c.SendStatus(fiber.StatusBadRequest)
		},
		"lịch học":  func(c *fiber.Ctx) error { return scheduleFail(c, archived, "Failed", fiber.StatusBadRequest) },
		"điểm danh": func(c *fiber.Ctx) error { return attendanceFail(c, archived, "Failed", fiber.StatusBadRequest) },
		"bài tập":   func(c *fiber.Ctx) error { return assignmentWriteFail(c, archived) },
		"chấm điểm": func(c *fiber.Ctx) error {
			h := NewGradeHandler(stubGradeService{bulkErr: archived})
			c.Locals("user_id", uuid.New())
			return h.BulkCreateGrades(c)
		},
	}
	bulk := `{"grades":[{"student_id":"` + uuid.NewString() + `","grade_type":"assignment","title":"Bài 1","score":8,"max_score":10}]}`
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := w3Do(t, h, "POST", bulk)
			if status != fiber.StatusConflict || body.Code != "CLASS_ARCHIVED" || body.Message == "" {
				t.Errorf("status=%d body=%+v, muốn 409 + code CLASS_ARCHIVED + message", status, body)
			}
		})
	}
}

// gradeErrorStatus so sánh bằng ==: lỗi bọc (fmt.Errorf %w) không khớp; ErrClassArchived trả thẳng từ service nên khớp.
func TestW3BE_GradeErrorStatus_ClassArchived409(t *testing.T) {
	if got := gradeErrorStatus(service.ErrClassArchived); got != fiber.StatusConflict {
		t.Errorf("ErrClassArchived -> %d, muốn 409", got)
	}
}

// Livestream/analytics: người không xem được lớp -> 404 (không lộ buổi live/id lớp), người xem được mà không có quyền
// -> 403 (ErrNotClassTeacher), như các route lớp khác.
func TestW3BE_LivestreamVaAnalytics_NguoiNgoai404_ThanhVien403(t *testing.T) {
	cases := []struct {
		name    string
		handler func(err error) fiber.Handler
	}{
		{"livestream", func(err error) fiber.Handler {
			return func(c *fiber.Ctx) error { return respondForbiddenOrError(c, err) }
		}},
		{"analytics", func(err error) fiber.Handler {
			return func(c *fiber.Ctx) error { return requireAnalyticsAuthErr(c, err) }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if status, _ := w3Do(t, tc.handler(service.ErrClassNotFound), "GET", ""); status != fiber.StatusNotFound {
				t.Errorf("người ngoài lớp: %d, muốn 404", status)
			}
			if status, _ := w3Do(t, tc.handler(service.ErrNotClassTeacher), "GET", ""); status != fiber.StatusForbidden {
				t.Errorf("thành viên lớp không có quyền: %d, muốn 403", status)
			}
			// Bọc lỗi (service trả fmt.Errorf %w) vẫn khớp.
			wrapped := errors.Join(errors.New("ctx"), service.ErrClassNotFound)
			if status, _ := w3Do(t, tc.handler(wrapped), "GET", ""); status != fiber.StatusNotFound {
				t.Errorf("ErrClassNotFound bọc: %d, muốn 404", status)
			}
		})
	}
}
