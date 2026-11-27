package handler

// Q11-ADMIN-03: GET /api/users nhận status=self_deleted (cùng active/locked) và chuyển nguyên giá trị xuống service;
// status lạ vẫn 400. Bỏ "self_deleted" khỏi danh sách hợp lệ thì ca self_deleted ĐỎ (400).

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

type qa11ListSvc struct {
	service.UserAdminServiceInterface
	gotStatus *string
}

func (s qa11ListSvc) ListUsers(_ context.Context, f repository.AdminUserListFilter) (*dto.AdminUserListResponseDTO, error) {
	*s.gotStatus = f.Status
	return &dto.AdminUserListResponseDTO{}, nil
}

func TestQA11_AdminUsersList_StatusHopLe(t *testing.T) {
	for _, tc := range []struct {
		query  string
		status int
		want   string
	}{
		{"status=self_deleted", 200, "self_deleted"},
		{"status=locked", 200, "locked"},
		{"status=active", 200, "active"},
		{"", 200, ""},
		{"status=deleted", 400, ""},
		{"status=SELF_DELETED", 400, ""},
	} {
		var got string
		app := fiber.New()
		app.Get("/users", NewUserAdminHandler(qa11ListSvc{gotStatus: &got}).GetUsers)
		resp, err := app.Test(httptest.NewRequest("GET", "/users?"+tc.query, nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != tc.status || got != tc.want {
			t.Errorf("%q: status=%d service-status=%q, muốn %d %q", tc.query, resp.StatusCode, got, tc.status, tc.want)
		}
	}
}
