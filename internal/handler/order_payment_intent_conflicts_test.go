package handler

// Integration QA2 (review M1): POST /orders/:id/payment-intent kiểm lại luật voucher "đơn hàng đầu tiên" lúc cấp mã
// thanh toán, và lỗi đó phải ra 409 kèm mã ổn định, không rơi xuống 400 ERR_CREATE_PAYMENT.

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

func TestCreatePaymentIntent_BusinessConflictsAre409WithStableCode(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"first-order voucher no longer eligible", service.ErrVoucherFirstOrderOnly, "VOUCHER_FIRST_ORDER_ONLY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewOrderHandler(nil, fakePaymentServiceErr{err: tc.err}, nil)
			app := fiber.New()
			app.Post("/orders/:id/payment-intent", func(c *fiber.Ctx) error {
				c.Locals("user_id", uuid.New())
				return h.CreatePaymentIntent(c)
			})
			req := httptest.NewRequest("POST", "/orders/"+uuid.NewString()+"/payment-intent",
				strings.NewReader(`{"payment_method":"qr_transfer"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			var body map[string]interface{}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if resp.StatusCode != fiber.StatusConflict || body["code"] != tc.wantCode {
				t.Fatalf("status=%d body=%v, muốn 409 code=%s", resp.StatusCode, body, tc.wantCode)
			}
		})
	}
}
