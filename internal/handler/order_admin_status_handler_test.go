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

// Review #76 MINOR: transaction_ref chỉ gồm khoảng trắng cũng là "thiếu mã giao dịch" → 400 ở tầng
// request (service nil: lọt xuống service là panic/đỏ).
func TestAdminRefundOrder_WhitespaceTransactionRefIsRejected(t *testing.T) {
	h := NewAdminOrderHandler(nil, nil, nil)
	app := fiber.New()
	app.Post("/orders/admin/:id/refund", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.RefundOrder(c)
	})

	req := httptest.NewRequest("POST", "/orders/admin/"+uuid.NewString()+"/refund",
		strings.NewReader(`{"reason":"QA","refund_method":"manual_bank_transfer","transaction_ref":"  \t "}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("transaction_ref toàn khoảng trắng trả %d, muốn 400", resp.StatusCode)
	}
}

// fakeOrderServiceInProgress — chỉ CreateOrder được gọi trong test này.
type fakeOrderServiceInProgress struct {
	service.OrderServiceInterface
}

func (fakeOrderServiceInProgress) CreateOrder(ctx context.Context, userID uuid.UUID, req dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	return nil, service.ErrOrderInProgress
}

// fakePaymentServiceExpired — chỉ CreatePaymentIntent được gọi trong test này.
type fakePaymentServiceExpired struct {
	service.PaymentServiceInterface
}

func (fakePaymentServiceExpired) CreatePaymentIntent(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, paymentMethod string) (*dto.PaymentIntentResponse, error) {
	return nil, service.ErrOrderExpired
}

// Review #76 MAJOR 2: đơn quá hạn giữ → 409 {"code":"ERR_ORDER_EXPIRED","message":<tiếng Việt>},
// không phải 400 ERR_CREATE_PAYMENT chung chung.
func TestCreatePaymentIntent_OrderExpiredIsConflict(t *testing.T) {
	h := NewOrderHandler(nil, fakePaymentServiceExpired{}, nil)
	app := fiber.New()
	app.Post("/orders/:id/payment-intent", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.CreatePaymentIntent(c)
	})

	req := httptest.NewRequest("POST", "/orders/"+uuid.NewString()+"/payment-intent", strings.NewReader(`{"payment_method":"qr_transfer"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("ErrOrderExpired trả %d, muốn 409", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "ERR_ORDER_EXPIRED" || body["message"] != service.ErrOrderExpired.Error() {
		t.Fatalf("body = %v", body)
	}
}

// fakePaymentServiceErr / fakeOrderServiceCancelErr — trả đúng lỗi cấu hình sẵn.
type fakePaymentServiceErr struct {
	service.PaymentServiceInterface
	err error
}

func (f fakePaymentServiceErr) CreatePaymentIntent(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, paymentMethod string) (*dto.PaymentIntentResponse, error) {
	return nil, f.err
}

type fakeOrderServiceCancelErr struct {
	service.OrderServiceInterface
	err error
}

func (f fakeOrderServiceCancelErr) CancelOrder(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, reason string) error {
	return f.err
}

// Review #76 vòng 3: đơn đã thanh toán / đang đối chiếu ngân hàng → 409 với code riêng ở CẢ
// payment-intent lẫn huỷ đơn, để web hiện đúng thông báo thay vì "tạo đơn mới".
func TestPaymentStateConflicts_AreMappedFor409(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"đã thanh toán":        {service.ErrPaymentAlreadyDone, "ERR_ORDER_ALREADY_PAID"},
		"đang đối chiếu":       {service.ErrPaymentVerificationPending, "ERR_PAYMENT_VERIFYING"},
		"hết hạn (giữ nguyên)": {service.ErrOrderExpired, "ERR_ORDER_EXPIRED"},
	}
	for name, c := range cases {
		for _, route := range []string{"payment-intent", "cancel"} {
			t.Run(name+"/"+route, func(t *testing.T) {
				h := NewOrderHandler(fakeOrderServiceCancelErr{err: c.err}, fakePaymentServiceErr{err: c.err}, nil)
				app := fiber.New()
				app.Post("/orders/:id/payment-intent", func(fc *fiber.Ctx) error {
					fc.Locals("user_id", uuid.New())
					return h.CreatePaymentIntent(fc)
				})
				app.Post("/orders/:id/cancel", func(fc *fiber.Ctx) error {
					fc.Locals("user_id", uuid.New())
					return h.CancelOrder(fc)
				})
				req := httptest.NewRequest("POST", "/orders/"+uuid.NewString()+"/"+route, strings.NewReader(`{"payment_method":"qr_transfer","reason":"QA"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				if err != nil {
					t.Fatalf("app.Test: %v", err)
				}
				if resp.StatusCode != fiber.StatusConflict {
					t.Fatalf("%v trả %d, muốn 409", c.err, resp.StatusCode)
				}
				var body map[string]interface{}
				_ = json.NewDecoder(resp.Body).Decode(&body)
				if body["code"] != c.code || body["message"] == "" || body["message"] == nil {
					t.Fatalf("body = %v, muốn code=%s kèm message", body, c.code)
				}
			})
		}
	}
}

type fakeOrderServiceCreateErr struct {
	service.OrderServiceInterface
	err error
}

func (f fakeOrderServiceCreateErr) CreateOrder(ctx context.Context, userID uuid.UUID, req dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	return nil, f.err
}

// Review #76 vòng 4: đơn trước cùng khoá đã cấp mã, đang đối chiếu (kể cả vừa huỷ) → POST /orders
// trả 409 ERR_PAYMENT_VERIFYING kèm câu riêng, không phải 400 ERR_CREATE_ORDER.
func TestCreateOrder_PreviousOrderVerifyingIsConflict(t *testing.T) {
	h := NewOrderHandler(fakeOrderServiceCreateErr{err: service.ErrPreviousOrderVerifying}, nil, nil)
	app := fiber.New()
	app.Post("/orders", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.CreateOrder(c)
	})
	req := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"source":"buy_now","course_ids":["`+uuid.NewString()+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("trả %d, muốn 409", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "ERR_PAYMENT_VERIFYING" || body["message"] != service.ErrPreviousOrderVerifying.Error() {
		t.Fatalf("body = %v", body)
	}
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
