package handler

// QA4 contracts 1-3: mã lỗi ổn định web dùng để hiện câu tiếng Việt. Bỏ ánh xạ ở assignmentWriteFail / scheduleFail thì
// test ĐỎ.

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
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
