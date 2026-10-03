package handler

// A-23 (QA hồi quy 03/10): đơn của người khác là đơn "không xem được" nên phải trả 404 như id không
// tồn tại. Trước đây trả 403 "forbidden: not the order owner" nên người lạ biết đơn đó có thật.

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

type fakeOrderServiceNotOwner struct {
	service.OrderServiceInterface
}

func (fakeOrderServiceNotOwner) GetOrderByID(ctx context.Context, orderID, userID uuid.UUID, isAdmin bool) (*dto.OrderResponse, error) {
	return nil, service.ErrOrderForbidden
}

func (fakeOrderServiceNotOwner) CancelOrder(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, reason string) error {
	return service.ErrOrderForbidden
}

func TestOrderRoutes_NotOwnerIsNotFound(t *testing.T) {
	h := NewOrderHandler(fakeOrderServiceNotOwner{}, fakePaymentServiceErr{err: service.ErrOrderForbidden}, nil)
	app := fiber.New()
	withUser := func(handler fiber.Handler) fiber.Handler {
		return func(c *fiber.Ctx) error {
			c.Locals("user_id", uuid.New())
			return handler(c)
		}
	}
	app.Get("/orders/:id", withUser(h.GetOrder))
	app.Post("/orders/:id/cancel", withUser(h.CancelOrder))
	app.Post("/orders/:id/payment-intent", withUser(h.CreatePaymentIntent))

	routes := []struct{ method, path string }{
		{"GET", ""},
		{"POST", "/cancel"},
		{"POST", "/payment-intent"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			req := httptest.NewRequest(r.method, "/orders/"+uuid.NewString()+r.path,
				strings.NewReader(`{"payment_method":"qr_transfer","reason":"QA"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if resp.StatusCode != fiber.StatusNotFound {
				t.Fatalf("đơn của người khác trả %d, muốn 404", resp.StatusCode)
			}
			var body map[string]interface{}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if body["code"] != "ERR_NOT_FOUND" {
				t.Fatalf("body = %v, muốn code=ERR_NOT_FOUND", body)
			}
			if msg, _ := body["message"].(string); strings.Contains(strings.ToLower(msg), "owner") {
				t.Fatalf("message không được lộ lý do phân quyền: %q", msg)
			}
		})
	}
}
