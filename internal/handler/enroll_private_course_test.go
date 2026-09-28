package handler

// Re-review vòng 2 PR #79 (D4): tự ghi danh khoá chưa xuất bản (hoặc không tồn tại) trả 404, như
// GET /courses/:id — trước đây 201 (khoá nháp giá 0) hoặc 400 (khoá không tồn tại).

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type hiddenEnrollSvc struct {
	service.EnrollmentServiceInterface
}

func (hiddenEnrollSvc) Enroll(context.Context, uuid.UUID, uuid.UUID) (*dto.EnrollmentResponseDTO, error) {
	return nil, service.ErrCourseHidden
}

func TestEnrollHandler_HiddenCourseIs404(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Post("/courses/:courseId/enroll", NewEnrollmentHandler(hiddenEnrollSvc{}, nil).Enroll)
	res, err := app.Test(httptest.NewRequest("POST", "/courses/"+uuid.NewString()+"/enroll", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusNotFound {
		t.Fatalf("ghi danh khoá bị ẩn = %d, muốn 404", res.StatusCode)
	}
}
