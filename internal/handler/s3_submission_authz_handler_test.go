package handler

// Lane S3 (nộp bài): service báo người gọi không xem được assignment (ErrAssignmentNotFound) thì
// Submit, RunCode và RunCustomCode phải trả 404, không phải 500 hay 200. Service giả nhúng interface
// nil để gọi nhầm method là panic. Bỏ ánh xạ lỗi này ở handler thì test ĐỎ.

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type s3SubmissionSvc struct {
	service.SubmissionServiceInterface
	err error
}

func (s *s3SubmissionSvc) Submit(context.Context, bool, dto.CreateSubmissionDTO) (*model.Submission, error) {
	return &model.Submission{}, s.err
}
func (s *s3SubmissionSvc) RunCode(context.Context, uuid.UUID, bool, dto.RunCodeDTO) (*dto.RunCodeResponseDTO, error) {
	return &dto.RunCodeResponseDTO{}, s.err
}
func (s *s3SubmissionSvc) RunCustomCode(context.Context, uuid.UUID, bool, dto.RunCustomCodeDTO) (*dto.RunCodeResponseDTO, error) {
	return &dto.RunCodeResponseDTO{}, s.err
}

func TestS3_SubmissionHandler_KhongXemDuocAssignmentThi404(t *testing.T) {
	aid := uuid.NewString()
	bodies := map[string]string{
		"/submissions":            `{"assignment_id":"` + aid + `","language":"python","code":"print(1)"}`,
		"/submissions/run":        `{"assignment_id":"` + aid + `","language":"python","code":"print(1)"}`,
		"/submissions/run-custom": `{"assignment_id":"` + aid + `","language":"python","code":"print(1)","custom_input":"1"}`,
	}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"không xem được assignment", service.ErrAssignmentNotFound, fiber.StatusNotFound},
		{"xem được", nil, fiber.StatusOK},
	} {
		for path, body := range bodies {
			t.Run(tc.name+" "+path, func(t *testing.T) {
				app := fiber.New()
				app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
				h := NewSubmissionHandler(&s3SubmissionSvc{err: tc.err}, nil)
				app.Post("/submissions", h.Submit)
				app.Post("/submissions/run", h.RunCode)
				app.Post("/submissions/run-custom", h.RunCustomCode)
				req := httptest.NewRequest("POST", path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				res, err := app.Test(req, -1)
				if err != nil {
					t.Fatal(err)
				}
				want := tc.want
				if tc.err == nil && path == "/submissions" {
					want = fiber.StatusCreated
				}
				if res.StatusCode != want {
					t.Fatalf("status=%d, muốn %d", res.StatusCode, want)
				}
			})
		}
	}
}
