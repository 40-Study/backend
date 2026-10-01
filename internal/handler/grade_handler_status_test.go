package handler

// Lane L2: mã HTTP của bảng điểm theo "xem được lớp hay không". Người không xem được lớp -> 404 (không dò
// được id lớp); học viên trong lớp (xem được, không được chấm) -> 403. Chấm hàng loạt trước đây trả 400 cho
// mọi lỗi nên hai mã này không bao giờ tới được client ở route đó.

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type stubGradeService struct {
	service.GradeServiceInterface
	bulkErr error
}

func (s stubGradeService) BulkCreateGrades(_ context.Context, _, _ uuid.UUID, _ dto.BulkCreateGradesDTO) ([]dto.GradeResponseDTO, error) {
	return nil, s.bulkErr
}

func TestGradeErrorStatus_404ChoNguoiKhongXemDuocLop_403ChoNguoiXemDuoc(t *testing.T) {
	if got := gradeErrorStatus(service.ErrClassNotFound); got != fiber.StatusNotFound {
		t.Errorf("ErrClassNotFound -> %d, muốn 404", got)
	}
	if got := gradeErrorStatus(service.ErrNotClassTeacher); got != fiber.StatusForbidden {
		t.Errorf("ErrNotClassTeacher -> %d, muốn 403", got)
	}
	if got := gradeErrorStatus(errors.New("lỗi khác")); got != 0 {
		t.Errorf("lỗi lạ -> %d, muốn 0 (giữ xử lý 400/500 sẵn có)", got)
	}
}

func TestBulkCreateGrades_MapPhanQuyenSangMaHTTP(t *testing.T) {
	body := `{"grades":[{"student_id":"` + uuid.NewString() + `","grade_type":"assignment","title":"Bài 1","score":8,"max_score":10}]}`
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"người ngoài", service.ErrClassNotFound, fiber.StatusNotFound},
		{"học viên trong lớp", service.ErrNotClassTeacher, fiber.StatusForbidden},
		{"học viên ngoài lớp (lỗi dữ liệu)", service.ErrStudentNotInClass, fiber.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := fiber.New()
			h := NewGradeHandler(stubGradeService{bulkErr: c.err})
			app.Post("/classes/:classId/grades/bulk", func(ctx *fiber.Ctx) error {
				ctx.Locals("user_id", uuid.New())
				return h.BulkCreateGrades(ctx)
			})
			req := httptest.NewRequest("POST", "/classes/"+uuid.NewString()+"/grades/bulk", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != c.want {
				t.Errorf("HTTP %d, muốn %d", resp.StatusCode, c.want)
			}
		})
	}
}
