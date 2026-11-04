package handler

// Q8 (QA3): huỷ đơn có mã chuyển khoản phân biệt hai tình huống chưa xác minh được tiền:
//   - ngân hàng không trả lời (chủ đơn đã báo chuyển khoản) -> 503 ERR_PAYMENT_PROVIDER_UNAVAILABLE
//   - ngân hàng trả lời nhưng chưa khớp / đang ân hạn      -> 409 ERR_PAYMENT_VERIFYING (giữ nguyên)

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

func TestCancelOrder_PaymentUnverifiedErrorsMapToDistinctCodes(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{service.ErrPaymentProviderUnavailable, fiber.StatusServiceUnavailable, "ERR_PAYMENT_PROVIDER_UNAVAILABLE"},
		{service.ErrPaymentVerificationPending, fiber.StatusConflict, "ERR_PAYMENT_VERIFYING"},
		{service.ErrPaymentAlreadyDone, fiber.StatusConflict, "ERR_ORDER_ALREADY_PAID"},
	}
	for _, tc := range cases {
		h := NewOrderHandler(w2aOrderSvc{err: tc.err}, w2aPaymentSvc{}, nil)
		app := fiber.New()
		app.Post("/orders/:id/cancel", func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return h.CancelOrder(c) })
		res, err := app.Test(httptest.NewRequest("POST", "/orders/"+uuid.NewString()+"/cancel", strings.NewReader(`{}`)), -1)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if res.StatusCode != tc.wantStatus || body["code"] != tc.wantCode {
			t.Errorf("%v: got %d %s, muốn %d %s", errors.Unwrap(tc.err), res.StatusCode, raw, tc.wantStatus, tc.wantCode)
		}
		if msg, _ := body["message"].(string); msg == "" || !strings.ContainsAny(msg, "ưạếộ") {
			t.Errorf("%s: thông báo phải là tiếng Việt, nhận %q", tc.wantCode, msg)
		}
	}
}
