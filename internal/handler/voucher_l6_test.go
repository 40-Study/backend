package handler

// L6 mục 6 và 8 ở tầng HTTP.

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type unsaveStub struct {
	service.VoucherServiceInterface
	unsaveErr error
	listLimit int
}

func (s *unsaveStub) UnsaveVoucher(context.Context, uuid.UUID, uuid.UUID) error { return s.unsaveErr }

// GetAllVouchers giả lập chuẩn hoá của service: limit ngoài 1-100 về 20.
func (s *unsaveStub) GetAllVouchers(_ context.Context, req *dto.GetVouchersRequest) ([]*model.Voucher, int64, error) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}
	s.listLimit = req.Limit
	return nil, 0, nil
}

func call(t *testing.T, app *fiber.App, method, path string) (int, map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(method, path, nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestUnsaveVoucher_HoldersOnlyReturns403WithCode(t *testing.T) {
	stub := &unsaveStub{unsaveErr: service.ErrHoldersOnlyVoucherNotRemovable}
	h := NewVoucherHandler(stub)
	app := fiber.New()
	app.Delete("/vouchers/:id/save", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.UnsaveVoucher(c)
	})
	status, body := call(t, app, "DELETE", "/vouchers/"+uuid.NewString()+"/save")
	if status != fiber.StatusForbidden || body["code"] != "VOUCHER_HOLDERS_ONLY_NOT_REMOVABLE" {
		t.Fatalf("status=%d body=%v, muốn 403 VOUCHER_HOLDERS_ONLY_NOT_REMOVABLE", status, body)
	}

	// Voucher công khai (service không báo lỗi) vẫn bỏ lưu được.
	stub.unsaveErr = nil
	if status, _ := call(t, app, "DELETE", "/vouchers/"+uuid.NewString()+"/save"); status != fiber.StatusOK {
		t.Fatalf("bỏ lưu bình thường: status=%d, muốn 200", status)
	}
}

// Người không giữ voucher (hoặc voucher không tồn tại) nhận cùng 404, không phải 403 "dành riêng": 403 sẽ xác
// nhận một UUID là voucher holders_only cho người lạ.
func TestUnsaveVoucher_NotHolderGets404(t *testing.T) {
	stub := &unsaveStub{unsaveErr: service.ErrUserVoucherNotFound}
	h := NewVoucherHandler(stub)
	app := fiber.New()
	app.Delete("/vouchers/:id/save", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.UnsaveVoucher(c)
	})
	status, body := call(t, app, "DELETE", "/vouchers/"+uuid.NewString()+"/save")
	if status != fiber.StatusNotFound || body["code"] != "ERR_VOUCHER_NOT_FOUND" {
		t.Fatalf("status=%d body=%v, muốn 404 ERR_VOUCHER_NOT_FOUND", status, body)
	}
}

// Danh sách admin trả limit/offset ĐÃ chuẩn hoá để web phân trang theo (không phải số client gửi).
func TestGetAllVouchers_EchoesNormalisedLimit(t *testing.T) {
	stub := &unsaveStub{}
	h := NewVoucherHandler(stub)
	app := fiber.New()
	app.Get("/vouchers", h.GetAllVouchers)
	_, body := call(t, app, "GET", "/vouchers?limit=500&offset=40")
	if body["limit"] != float64(20) || body["offset"] != float64(40) {
		t.Fatalf("limit/offset = %v/%v, muốn 20/40 (limit 500 bị chuẩn hoá)", body["limit"], body["offset"])
	}
}
