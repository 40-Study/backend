package handler

// respondCodeEditorError (contract sec. 1): mỗi sentinel -> đúng HTTP status và {code, error, message}; lỗi lạ ->
// 500 INTERNAL_ERROR. Body KHÔNG BAO GIỜ chứa URL/host/cổng hay chữ lỗi Go (chi tiết chỉ nằm trong log).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/service"
)

func codeEditorErrorBody(t *testing.T, err error) (int, string, map[string]string) {
	t.Helper()
	app := fiber.New()
	app.Get("/x", func(c *fiber.Ctx) error { return respondCodeEditorError(c, err) })
	resp, terr := app.Test(httptest.NewRequest("GET", "/x", nil))
	if terr != nil {
		t.Fatalf("app.Test: %v", terr)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body map[string]string
	if jerr := json.Unmarshal(raw, &body); jerr != nil {
		t.Fatalf("body không phải {code,error,message}: %s", raw)
	}
	return resp.StatusCode, string(raw), body
}

func TestRespondCodeEditorError_SentinelMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{service.ErrUnsupportedLanguage, 400, "UNSUPPORTED_LANGUAGE"},
		{service.ErrNoTestCases, 400, "NO_TEST_CASES"},
		{service.ErrInvalidScope, 400, "INVALID_SCOPE"},
		{service.ErrDraftLimitReached, 409, "DRAFT_LIMIT_REACHED"},
		{service.ErrCodeTooLarge, 413, "CODE_TOO_LARGE"},
		{service.ErrJudgeUnavailable, 503, "JUDGE_UNAVAILABLE"},
		{errors.New("pq: relation does not exist"), 500, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		// Sentinel bị bọc (fmt.Errorf %w) vẫn phải nhận ra.
		status, _, body := codeEditorErrorBody(t, fmt.Errorf("lớp ngoài: %w", tc.err))
		if status != tc.status || body["code"] != tc.code {
			t.Errorf("%v -> %d %q, muốn %d %q", tc.err, status, body["code"], tc.status, tc.code)
		}
		if body["error"] != strings.ToLower(tc.code) {
			t.Errorf("%s: error = %q, muốn %q", tc.code, body["error"], strings.ToLower(tc.code))
		}
		if strings.TrimSpace(body["message"]) == "" {
			t.Errorf("%s: thiếu message tiếng Việt", tc.code)
		}
		if len(body) != 3 {
			t.Errorf("%s: body có key thừa/thiếu: %v", tc.code, body)
		}
	}
	_, _, body := codeEditorErrorBody(t, service.ErrJudgeUnavailable)
	if body["message"] != "Máy chấm tạm thời không khả dụng, vui lòng thử lại sau." {
		t.Errorf("JUDGE_UNAVAILABLE message = %q", body["message"])
	}
}

func TestRespondCodeEditorError_NeverLeaksTransportDetail(t *testing.T) {
	leaky := []error{
		fmt.Errorf("%w: Post \"http://localhost:2358/submissions?wait=true\": dial tcp 127.0.0.1:2358: connect: connection refused", service.ErrJudgeUnavailable),
		errors.New(`Post "http://localhost:2358/submissions": connection refused`),
		fmt.Errorf("judge0 rejected request (HTTP 422): {\"source_code\":[\"can't be blank\"]} at http://judge0:2358"),
	}
	for _, err := range leaky {
		_, raw, _ := codeEditorErrorBody(t, err)
		for _, bad := range []string{"http", "2358", "localhost", "judge0", "dial", "refused", "422"} {
			if strings.Contains(strings.ToLower(raw), bad) {
				t.Errorf("body lộ %q: %s", bad, raw)
			}
		}
	}
}
