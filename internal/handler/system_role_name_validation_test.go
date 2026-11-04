package handler

// QA3-03: POST/PUT /api/system-roles chấp nhận tên rỗng / toàn khoảng trắng / 1 ký tự vì handler không gọi
// ValidateStruct. Tên hợp lệ phải được TRIM trước khi tới service; tên sai là 400 tiếng Việt và service không bị gọi.

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type recordingSystemRoleSvc struct {
	service.SystemRoleServiceInterface
	created []dto.CreateSystemRoleDTO
	updated []dto.UpdateSystemRoleDTO
}

func (r *recordingSystemRoleSvc) CreateSystemRole(_ context.Context, req dto.CreateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error) {
	r.created = append(r.created, req)
	return &dto.SystemRoleResponseDTO{ID: uuid.New(), Name: req.Name}, nil
}

func (r *recordingSystemRoleSvc) UpdateSystemRole(_ context.Context, id uuid.UUID, req dto.UpdateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error) {
	r.updated = append(r.updated, req)
	return &dto.SystemRoleResponseDTO{ID: id}, nil
}

func systemRoleApp(svc *recordingSystemRoleSvc) *fiber.App {
	h := NewSystemRoleHandler(svc)
	app := fiber.New()
	app.Post("/system-roles", h.CreateSystemRole)
	app.Put("/system-roles/:id", h.UpdateSystemRole)
	return app
}

func sendSystemRoleJSON(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestSystemRole_InvalidNames_Return400Vietnamese(t *testing.T) {
	bad := map[string]string{
		"rỗng":           `{"name":""}`,
		"thiếu tên":      `{}`,
		"khoảng trắng":   `{"name":"   "}`,
		"một ký tự":      `{"name":"a"}`,
		"một ký tự+cách": `{"name":"  a  "}`,
		"quá dài":        `{"name":"` + strings.Repeat("x", 101) + `"}`,
	}
	for label, body := range bad {
		svc := &recordingSystemRoleSvc{}
		app := systemRoleApp(svc)
		status, raw := sendSystemRoleJSON(t, app, "POST", "/system-roles", body)
		if status != 400 {
			t.Errorf("POST %s: status %d, muốn 400 (%s)", label, status, raw)
		}
		var resp struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal([]byte(raw), &resp)
		if !strings.Contains(resp.Message, "Tên vai trò") {
			t.Errorf("POST %s: message %q phải là tiếng Việt về tên vai trò", label, resp.Message)
		}
		if len(svc.created) != 0 {
			t.Errorf("POST %s: service vẫn bị gọi với %+v", label, svc.created)
		}
	}

	// PUT: trường name có mặt thì cũng phải hợp lệ (kể cả chuỗi rỗng, thứ mà tag omitempty của con trỏ bỏ qua).
	for label, body := range bad {
		if label == "thiếu tên" {
			continue // PUT không gửi name = giữ nguyên tên cũ, hợp lệ
		}
		svc := &recordingSystemRoleSvc{}
		status, raw := sendSystemRoleJSON(t, systemRoleApp(svc), "PUT", "/system-roles/"+uuid.NewString(), body)
		if status != 400 || len(svc.updated) != 0 {
			t.Errorf("PUT %s: status %d, service gọi %d lần, muốn 400 và 0 lần (%s)", label, status, len(svc.updated), raw)
		}
	}
}

func TestSystemRole_ValidNames_AreTrimmedBeforeService(t *testing.T) {
	svc := &recordingSystemRoleSvc{}
	app := systemRoleApp(svc)

	if status, raw := sendSystemRoleJSON(t, app, "POST", "/system-roles", `{"name":"  Trợ giảng  ","description":"  mô tả  "}`); status != 201 {
		t.Fatalf("POST hợp lệ: status %d (%s)", status, raw)
	}
	if len(svc.created) != 1 || svc.created[0].Name != "Trợ giảng" || svc.created[0].Description != "mô tả" {
		t.Errorf("service nhận %+v, muốn tên/mô tả đã trim", svc.created)
	}

	if status, raw := sendSystemRoleJSON(t, app, "PUT", "/system-roles/"+uuid.NewString(), `{"name":"  Giám sát  "}`); status != 200 {
		t.Fatalf("PUT hợp lệ: status %d (%s)", status, raw)
	}
	if len(svc.updated) != 1 || svc.updated[0].Name == nil || *svc.updated[0].Name != "Giám sát" {
		t.Errorf("service nhận %+v, muốn tên đã trim", svc.updated)
	}

	// PUT chỉ đổi mô tả (không gửi name) vẫn hợp lệ.
	if status, raw := sendSystemRoleJSON(t, app, "PUT", "/system-roles/"+uuid.NewString(), `{"description":"chỉ mô tả"}`); status != 200 {
		t.Fatalf("PUT không có name: status %d (%s)", status, raw)
	}
}
