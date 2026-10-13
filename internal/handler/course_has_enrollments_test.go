package handler

// QA2 Q2-01 / review M2: xoá khoá còn ghi danh hoạt động trả 409 COURSE_HAS_ENROLLMENTS (không phải 400 chung)
// kèm thông báo tiếng Việt gợi ý "Lưu trữ".

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type enrolledCourseSvc struct{ service.CourseServiceInterface }

func (enrolledCourseSvc) DeleteCourse(context.Context, uuid.UUID, uuid.UUID, bool) error {
	return service.ErrCourseHasEnrollments
}

func TestDeleteCourse_HasEnrollmentsReturns409WithCode(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Delete("/courses/:id", NewCourseHandler(enrolledCourseSvc{}, nil).DeleteCourse)

	resp, err := app.Test(httptest.NewRequest("DELETE", "/courses/"+uuid.NewString(), nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct{ Message, Code string }
	_ = json.Unmarshal(raw, &body)
	if resp.StatusCode != 409 || body.Code != "COURSE_HAS_ENROLLMENTS" || !strings.Contains(body.Message, "Lưu trữ") {
		t.Fatalf("status=%d body=%s, muốn 409 COURSE_HAS_ENROLLMENTS nhắc \"Lưu trữ\"", resp.StatusCode, raw)
	}
}
