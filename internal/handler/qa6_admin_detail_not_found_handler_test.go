package handler

// QA6-A06 (C8): admin GET đơn hàng / người dùng theo id: id sai định dạng -> 400 ERR_INVALID_ID, không có -> 404
// ERR_NOT_FOUND, lỗi khác (DB) -> 500, TẤT CẢ kèm message tiếng Việt. Trước đây mọi lỗi service đều thành 404 "Order not
// found" (cả lỗi DB) và id sai của người dùng không có code. Bỏ errors.Is(ErrOrderNotFound) thì ca 500 ĐỎ (404).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

type qa6OrderSvc struct {
	service.OrderServiceInterface
	err error
}

func (s qa6OrderSvc) GetOrderByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.OrderResponse, error) {
	return nil, s.err
}

type qa6UserSvc struct {
	service.UserAdminServiceInterface
	err error
}

func (s qa6UserSvc) GetUserDetail(context.Context, uuid.UUID) (*dto.AdminUserDetailDTO, error) {
	return nil, s.err
}

func qa6Get(t *testing.T, app *fiber.App, path string) (int, map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestQA6_AdminOrderDetail_MaLoi(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string
		err    error
		status int
		code   string
		msg    string
	}{
		{"id sai", "/o/not-a-uuid", nil, 400, "ERR_INVALID_ID", "Mã đơn hàng không hợp lệ."},
		{"không có", "/o/" + uuid.NewString(), service.ErrOrderNotFound, 404, "ERR_NOT_FOUND", "Không tìm thấy đơn hàng."},
		{"lỗi DB", "/o/" + uuid.NewString(), errors.New("connection refused"), 500, "ERR_INTERNAL", "Không tải được đơn hàng. Vui lòng thử lại."},
	} {
		app := fiber.New()
		app.Get("/o/:id", NewAdminOrderHandler(qa6OrderSvc{err: tc.err}, nil, nil).GetOrder)
		status, body := qa6Get(t, app, tc.path)
		if status != tc.status || body["code"] != tc.code || body["message"] != tc.msg {
			t.Errorf("%s: status=%d body=%v, muốn %d %s %q", tc.name, status, body, tc.status, tc.code, tc.msg)
		}
	}
}

func TestQA6_AdminUserDetail_MaLoi(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string
		err    error
		status int
		code   string
		msg    string
	}{
		{"id sai", "/u/not-a-uuid", nil, 400, "ERR_INVALID_ID", "Mã người dùng không hợp lệ."},
		{"không có", "/u/" + uuid.NewString(), repository.ErrAdminUserNotFound, 404, "ERR_NOT_FOUND", "Không tìm thấy người dùng."},
	} {
		app := fiber.New()
		app.Get("/u/:id", NewUserAdminHandler(qa6UserSvc{err: tc.err}).GetUser)
		status, body := qa6Get(t, app, tc.path)
		if status != tc.status || body["code"] != tc.code || body["message"] != tc.msg {
			t.Errorf("%s: status=%d body=%v, muốn %d %s %q", tc.name, status, body, tc.status, tc.code, tc.msg)
		}
	}
}
