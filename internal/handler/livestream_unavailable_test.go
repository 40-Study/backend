package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/service"
)

// Q3-13 (QA3): LiveKit chưa cấu hình -> 503 ERR_LIVE_UNAVAILABLE với câu tiếng Việt, kể cả khi lỗi bị bọc.
func TestRespondForbiddenOrError_LivekitNotConfiguredIs503(t *testing.T) {
	app := fiber.New()
	wrapped := fmt.Errorf("failed to create livekit room: %w", service.ErrLivekitNotConfigured)
	app.Post("/x", func(c *fiber.Ctx) error { return respondForbiddenOrError(c, wrapped) })
	res, err := app.Test(httptest.NewRequest("POST", "/x", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	if res.StatusCode != fiber.StatusServiceUnavailable || body["code"] != "ERR_LIVE_UNAVAILABLE" {
		t.Fatalf("got %d %s", res.StatusCode, raw)
	}
	msg, _ := body["message"].(string)
	if !strings.Contains(msg, "trực tuyến") || strings.Contains(strings.ToLower(msg), "api key") {
		t.Errorf("thông báo phải là tiếng Việt và không lộ chi tiết cấu hình: %q", msg)
	}
	if _, leaked := body["error"]; leaked {
		t.Errorf("không được trả chuỗi lỗi gốc: %s", raw)
	}
}
