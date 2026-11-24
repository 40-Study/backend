package handler

// QA4 contracts 1-3: mã lỗi ổn định web dùng để hiện câu tiếng Việt. Bỏ ánh xạ ở assignmentWriteFail / scheduleFail thì
// test ĐỎ.

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

func TestQA4_TeacherErrorCodes(t *testing.T) {
	cases := []struct {
		name   string
		fail   func(c *fiber.Ctx) error
		status int
		code   string
	}{
		{"T01 xoá bài đã có bài nộp", func(c *fiber.Ctx) error { return assignmentWriteFail(c, service.ErrAssignmentHasSubmissions) },
			fiber.StatusConflict, "ERR_ASSIGNMENT_HAS_SUBMISSIONS"},
		{"T08 hạn nộp đã qua", func(c *fiber.Ctx) error { return assignmentWriteFail(c, service.ErrDueDateInPast) },
			fiber.StatusBadRequest, "ERR_DUE_DATE_IN_PAST"},
		{"T07 buổi chưa tới ngày", func(c *fiber.Ctx) error {
			return scheduleFail(c, service.ErrSessionNotStarted, "Failed to mark attendance", fiber.StatusBadRequest)
		}, fiber.StatusUnprocessableEntity, "ERR_SESSION_NOT_STARTED"},
		{"QA5 R4 huỷ buổi đã bắt đầu", func(c *fiber.Ctx) error {
			return scheduleFail(c, service.ErrSessionAlreadyStarted, "Failed to cancel session", fiber.StatusBadRequest)
		}, fiber.StatusUnprocessableEntity, "ERR_SESSION_ALREADY_STARTED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", tc.fail)
			res, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.status || body["code"] != tc.code || body["message"] == "" {
				t.Fatalf("status=%d body=%v, muốn %d code=%s", res.StatusCode, body, tc.status, tc.code)
			}
		})
	}
}

// QA4-T06: sinh buổi mà mọi ngày đều bị bỏ qua -> 409, thân lỗi vẫn kèm skipped[] (web đọc skipped ?? []).
func TestQA4_GenerateSessions_AllSkippedConflictCarriesSkipped(t *testing.T) {
	svc := &s5ScheduleSvc{err: service.ErrSessionOverlap, skipped: []dto.SkippedSessionDTO{
		{Date: "2031-03-03", Reason: dto.SkippedSessionDuplicate}, {Date: "2031-03-10", Reason: dto.SkippedSessionCancelled}}}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Post("/classes/:classId/sessions/generate", NewScheduleHandler(svc, nil).GenerateSessions)
	req := httptest.NewRequest("POST", "/classes/"+uuid.NewString()+"/sessions/generate",
		strings.NewReader(`{"start_date":"2031-03-03","end_date":"2031-03-10"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Message string                  `json:"message"`
		Skipped []dto.SkippedSessionDTO `json:"skipped"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusConflict || len(body.Skipped) != 2 || body.Skipped[1].Reason != dto.SkippedSessionCancelled || body.Message == "" {
		t.Fatalf("status=%d body=%+v, muốn 409 kèm 2 ngày bị bỏ qua", res.StatusCode, body)
	}
}
