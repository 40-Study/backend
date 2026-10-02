package handler

// Lane L7: tạo lớp vào tổ chức mà người gọi không phải thành viên active phải ra 403 (xem được, không được
// phép), không phải 400. Trước đây không test nào ghim ánh xạ ErrNotOrgMember trong classErrorStatus: bỏ nó
// đi (rơi xuống nhánh 400 mặc định) mà toàn bộ test handler vẫn xanh. Lỗi được bọc `%w` như service thật làm.

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

func TestL7_ClassErrorStatus_ErrNotOrgMemberLa403(t *testing.T) {
	for _, err := range []error{service.ErrNotOrgMember, fmt.Errorf("tạo lớp: %w", service.ErrNotOrgMember)} {
		if got := classErrorStatus(err); got != fiber.StatusForbidden {
			t.Errorf("classErrorStatus(%v) = %d, muốn %d", err, got, fiber.StatusForbidden)
		}
	}
}

func TestL7_CreateClassHandler_KhongPhaiThanhVienToChuc403(t *testing.T) {
	svc := &s4ClassSvc{err: fmt.Errorf("tạo lớp: %w", service.ErrNotOrgMember)}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Post("/classes", NewClassHandler(svc, nil).CreateClass)

	req := httptest.NewRequest("POST", "/classes", strings.NewReader(`{"name":"x","organization_id":"`+uuid.NewString()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status=%d, muốn 403", res.StatusCode)
	}
}
