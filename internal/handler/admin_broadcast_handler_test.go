package handler

// Contract C3: envelope {"message","code"} cho lỗi, 201 {"message","data"} cho gửi, và lỗi hạ tầng không lộ chuỗi gốc.
// Route + handler THẬT, service giả.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type fakeBroadcastService struct {
	previewRes *dto.BroadcastPreviewDTO
	sendRes    *dto.BroadcastResultDTO
	err        error
	sent       []dto.BroadcastRequestDTO
}

func (f *fakeBroadcastService) Preview(_ context.Context, _ dto.BroadcastPreviewRequestDTO) (*dto.BroadcastPreviewDTO, error) {
	return f.previewRes, f.err
}

func (f *fakeBroadcastService) Send(_ context.Context, req dto.BroadcastRequestDTO) (*dto.BroadcastResultDTO, error) {
	f.sent = append(f.sent, req)
	return f.sendRes, f.err
}

func broadcastApp(svc service.BroadcastServiceInterface) (*fiber.App, *int64) {
	h := NewAdminBroadcastHandler(svc)
	delivered := new(int64)
	app := fiber.New()
	record := func(c *fiber.Ctx) error {
		err := c.Next()
		if v, ok := c.Locals(BroadcastDeliveredLocal).(int64); ok {
			*delivered = v
		}
		return err
	}
	app.Post("/preview", record, h.Preview)
	app.Post("/send", record, h.Send)
	return app, delivered
}

func broadcastPost(t *testing.T, app *fiber.App, path, body string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, string(raw)
}

func TestBroadcastHandler_PreviewVaSend_HinhDangResponse(t *testing.T) {
	svc := &fakeBroadcastService{
		previewRes: &dto.BroadcastPreviewDTO{RecipientCount: 123},
		sendRes:    &dto.BroadcastResultDTO{RecipientCount: 123, Audience: "roles", Roles: []string{"STUDENT"}, NotificationType: "system"},
	}
	app, delivered := broadcastApp(svc)

	status, body, _ := broadcastPost(t, app, "/preview", `{"audience":"roles","roles":["STUDENT"]}`)
	if status != 200 || body["data"].(map[string]any)["recipient_count"] != float64(123) {
		t.Fatalf("preview = %d %v; muốn 200 data.recipient_count=123", status, body)
	}

	status, body, _ = broadcastPost(t, app, "/send", `{"title":"Bảo trì","content":"22h","audience":"roles","roles":["STUDENT"]}`)
	data, _ := body["data"].(map[string]any)
	if status != 201 || body["message"] != "Đã gửi thông báo" || data["recipient_count"] != float64(123) ||
		data["audience"] != "roles" || data["notification_type"] != "system" {
		t.Fatalf("send = %d %v; muốn 201 theo contract C3", status, body)
	}
	if roles, _ := data["roles"].([]any); len(roles) != 1 || roles[0] != "STUDENT" {
		t.Fatalf("data.roles = %v; muốn [STUDENT]", data["roles"])
	}
	if *delivered != 123 {
		t.Fatalf("Locals %s = %d; muốn 123 cho middleware audit", BroadcastDeliveredLocal, *delivered)
	}
}

func TestBroadcastHandler_AudienceAll_RolesLaMangRong(t *testing.T) {
	svc := &fakeBroadcastService{sendRes: &dto.BroadcastResultDTO{RecipientCount: 2, Audience: "all", Roles: []string{}, NotificationType: "system"}}
	app, _ := broadcastApp(svc)
	_, _, raw := broadcastPost(t, app, "/send", `{"title":"a","content":"b","audience":"all"}`)
	if !strings.Contains(raw, `"roles":[]`) {
		t.Fatalf("audience=all phải trả roles là [] chứ không phải null/thiếu: %s", raw)
	}
}

func TestBroadcastHandler_ValidationBounds_400(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := map[string]string{
		"thiếu tiêu đề":     `{"content":"b","audience":"all"}`,
		"tiêu đề 256":       `{"title":"` + long(256) + `","content":"b","audience":"all"}`,
		"thiếu nội dung":    `{"title":"a","audience":"all"}`,
		"nội dung 2001":     `{"title":"a","content":"` + long(2001) + `","audience":"all"}`,
		"thiếu audience":    `{"title":"a","content":"b"}`,
		"audience lạ":       `{"title":"a","content":"b","audience":"everyone"}`,
		"loại không hợp lệ": `{"title":"a","content":"b","audience":"all","notification_type":"payment_failed"}`,
		"JSON hỏng":         `{"title":`,
	}
	for name, body := range cases {
		svc := &fakeBroadcastService{}
		app, _ := broadcastApp(svc)
		status, out, _ := broadcastPost(t, app, "/send", body)
		if status != 400 || out["code"] != "VALIDATION_FAILED" || out["message"] == "" {
			t.Errorf("%s: %d %v; muốn 400 VALIDATION_FAILED", name, status, out)
		}
		if len(svc.sent) != 0 {
			t.Errorf("%s: service vẫn được gọi", name)
		}
	}
	// Đúng biên vẫn qua.
	svc := &fakeBroadcastService{sendRes: &dto.BroadcastResultDTO{Roles: []string{}}}
	app, _ := broadcastApp(svc)
	if status, _, _ := broadcastPost(t, app, "/send", `{"title":"`+long(255)+`","content":"`+long(2000)+`","audience":"all","notification_type":"promotion"}`); status != 201 {
		t.Errorf("255/2000 ký tự = %d; muốn 201", status)
	}
	if status, out, _ := broadcastPost(t, app, "/preview", `{"audience":"nobody"}`); status != 400 || out["code"] != "VALIDATION_FAILED" {
		t.Errorf("preview audience lạ = %d %v; muốn 400 VALIDATION_FAILED", status, out)
	}
}

func TestBroadcastHandler_MaLoiNghiepVu(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&service.BroadcastError{Status: 400, Code: "UNKNOWN_ROLE", Message: "Vai trò không tồn tại: X"}, 400, "UNKNOWN_ROLE"},
		{&service.BroadcastError{Status: 422, Code: "NO_RECIPIENTS", Message: "Không có người nhận"}, 422, "NO_RECIPIENTS"},
	}
	for _, tc := range cases {
		app, _ := broadcastApp(&fakeBroadcastService{err: tc.err})
		status, out, _ := broadcastPost(t, app, "/send", `{"title":"a","content":"b","audience":"all"}`)
		if status != tc.status || out["code"] != tc.code || out["message"] != tc.err.Error() {
			t.Errorf("%s: %d %v", tc.code, status, out)
		}
	}
}

func TestBroadcastHandler_LoiGiuaChung_NeuSoDaGiao(t *testing.T) {
	err := &service.BroadcastPartialError{Delivered: 500, Cause: errors.New("pq: connection refused host=10.0.0.5")}
	app, delivered := broadcastApp(&fakeBroadcastService{err: err})
	status, out, raw := broadcastPost(t, app, "/send", `{"title":"a","content":"b","audience":"all"}`)
	if status != 500 || out["code"] != "BROADCAST_PARTIAL" || out["delivered"] != float64(500) || !strings.Contains(out["message"].(string), "500") {
		t.Fatalf("partial = %d %v; muốn 500 BROADCAST_PARTIAL nêu 500 người đã nhận", status, out)
	}
	if strings.Contains(raw, "10.0.0.5") || strings.Contains(raw, "pq:") {
		t.Fatalf("response lộ chi tiết lỗi hạ tầng: %s", raw)
	}
	if *delivered != 500 {
		t.Fatalf("Locals delivered = %d; muốn 500 để audit ghi được số đã giao", *delivered)
	}
}

func TestBroadcastHandler_LoiHaTang_KhongLoChuoiLoi(t *testing.T) {
	app, _ := broadcastApp(&fakeBroadcastService{err: errors.New("pq: relation users does not exist")})
	for _, path := range []string{"/send", "/preview"} {
		body := `{"title":"a","content":"b","audience":"all"}`
		status, out, raw := broadcastPost(t, app, path, body)
		if status != 500 || out["code"] != "INTERNAL_ERROR" || strings.Contains(raw, "pq:") {
			t.Errorf("%s: %d %s; muốn 500 INTERNAL_ERROR không lộ chuỗi lỗi", path, status, raw)
		}
	}
}
