package handler

// QA vòng 2, lane B: B7 (status lạ ở danh sách đơn admin → 400) và B4 (đơn trùng khoá đang thanh
// toán → 409 ở POST /orders).

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

func TestAdminListOrders_UnknownStatusIsBadRequest(t *testing.T) {
	// Service nil: nếu handler không chặn status lạ mà gọi xuống service thì test panic/đỏ.
	h := NewAdminOrderHandler(nil, nil, nil)
	app := fiber.New()
	app.Get("/orders/admin", h.ListOrders)

	resp, err := app.Test(httptest.NewRequest("GET", "/orders/admin?status=bogus", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status=bogus trả %d, muốn 400", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "invalid_status" {
		t.Fatalf("body = %v, muốn error=invalid_status", body)
	}
}

// B6: quyết định #1 yêu cầu hoàn tiền "kèm mã giao dịch" — thiếu transaction_ref phải bị chặn ở
// tầng request (service nil: lọt xuống service là panic/đỏ).
func TestAdminRefundOrder_RequiresTransactionRef(t *testing.T) {
	h := NewAdminOrderHandler(nil, nil, nil)
	app := fiber.New()
	app.Post("/orders/admin/:id/refund", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.RefundOrder(c)
	})

	req := httptest.NewRequest("POST", "/orders/admin/"+uuid.NewString()+"/refund",
		strings.NewReader(`{"reason":"QA","refund_method":"manual_bank_transfer"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("thiếu transaction_ref trả %d, muốn 400", resp.StatusCode)
	}
}

// fakeOrderServiceInProgress — chỉ CreateOrder được gọi trong test này.
type fakeOrderServiceInProgress struct {
	service.OrderServiceInterface
}

func (fakeOrderServiceInProgress) CreateOrder(ctx context.Context, userID uuid.UUID, req dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	return nil, service.ErrOrderInProgress
}

func TestCreateOrder_OrderInProgressIsConflict(t *testing.T) {
	h := NewOrderHandler(fakeOrderServiceInProgress{}, nil, nil)
	app := fiber.New()
	app.Post("/orders", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.CreateOrder(c)
	})

	req := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"source":"buy_now","course_ids":["`+uuid.NewString()+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("ErrOrderInProgress trả %d, muốn 409", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "ERR_ORDER_IN_PROGRESS" || body["message"] != service.ErrOrderInProgress.Error() {
		t.Fatalf("body = %v", body)
	}
}
