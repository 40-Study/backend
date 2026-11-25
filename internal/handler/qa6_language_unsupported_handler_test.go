package handler

// QA6 S6-01 (C11): chạy thử với ngôn ngữ máy chấm không chạy được -> 422 ERR_LANGUAGE_UNSUPPORTED (không phải 500).
// Bỏ respondLanguageUnsupported khỏi RunCode/RunCustomCode thì cả hai ca ĐỎ (500).

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type langUnsupportedSvc struct {
	service.SubmissionServiceInterface
}

var errDartUnsupported = &service.ContestError{Status: 422, Code: "ERR_LANGUAGE_UNSUPPORTED", Message: "Ngôn ngữ dart chưa hỗ trợ chấm tự động."}

func (langUnsupportedSvc) RunCode(context.Context, uuid.UUID, bool, dto.RunCodeDTO) (*dto.RunCodeResponseDTO, error) {
	return nil, errDartUnsupported
}
func (langUnsupportedSvc) RunCustomCode(context.Context, uuid.UUID, bool, dto.RunCustomCodeDTO) (*dto.RunCodeResponseDTO, error) {
	return nil, errDartUnsupported
}

func TestQA6_RunCode_NgonNguKhongHoTro422(t *testing.T) {
	h := NewSubmissionHandler(langUnsupportedSvc{}, nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Post("/run", h.RunCode)
	app.Post("/run-custom", h.RunCustomCode)
	for _, path := range []string{"/run", "/run-custom"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{"assignment_id":"`+uuid.NewString()+`","language":"dart","code":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if resp.StatusCode != 422 || body["code"] != "ERR_LANGUAGE_UNSUPPORTED" {
			t.Errorf("%s: status=%d body=%v, muốn 422 ERR_LANGUAGE_UNSUPPORTED", path, resp.StatusCode, body)
		}
	}
}

type langNotAllowedSvc struct {
	service.SubmissionServiceInterface
}

var errDartNotAllowed = &service.ContestError{Status: 400, Code: "ERR_LANGUAGE_NOT_ALLOWED", Message: "Bài này không cho phép ngôn ngữ dart."}

func (langNotAllowedSvc) Submit(context.Context, bool, dto.CreateSubmissionDTO) (*model.Submission, error) {
	return nil, errDartNotAllowed
}
func (langNotAllowedSvc) RunCode(context.Context, uuid.UUID, bool, dto.RunCodeDTO) (*dto.RunCodeResponseDTO, error) {
	return nil, errDartNotAllowed
}
func (langNotAllowedSvc) RunCustomCode(context.Context, uuid.UUID, bool, dto.RunCustomCodeDTO) (*dto.RunCodeResponseDTO, error) {
	return nil, errDartNotAllowed
}

// QA6 review M3: ngôn ngữ ngoài danh sách của bài -> 400 ERR_LANGUAGE_NOT_ALLOWED ở cả nộp bài, chạy thử và chạy tuỳ ý
// (không phải 500). Bỏ ERR_LANGUAGE_NOT_ALLOWED khỏi respondLanguageUnsupported hoặc khỏi Submit thì ca tương ứng ĐỎ.
func TestQA6_LanguageNotAllowed_400(t *testing.T) {
	h := NewSubmissionHandler(langNotAllowedSvc{}, nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Post("/submit", h.Submit)
	app.Post("/run", h.RunCode)
	app.Post("/run-custom", h.RunCustomCode)
	for _, path := range []string{"/submit", "/run", "/run-custom"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{"assignment_id":"`+uuid.NewString()+`","language":"dart","code":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if resp.StatusCode != 400 || body["code"] != "ERR_LANGUAGE_NOT_ALLOWED" {
			t.Errorf("%s: status=%d body=%v, muốn 400 ERR_LANGUAGE_NOT_ALLOWED", path, resp.StatusCode, body)
		}
	}
}
