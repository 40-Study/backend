package handler

// QA4-S7: ngân hàng không trả lời khi xác minh nạp xu -> 503 ERR_PAYMENT_PROVIDER_UNAVAILABLE (không phải 400
// kèm lỗi hạ tầng). Mutation đã kiểm: bỏ nhánh errors.Is(ErrCoinBankUnavailable) làm test ĐỎ (400).

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type coinVerifyDownStub struct{ service.CoinServiceInterface }

func (coinVerifyDownStub) VerifyPurchase(context.Context, uuid.UUID, uuid.UUID) (*dto.CoinPurchaseResponse, error) {
	return nil, service.ErrCoinBankUnavailable
}

func TestCoinVerify_BankUnavailableReturns503(t *testing.T) {
	app := fiber.New()
	user := uuid.New()
	app.Post("/coins/purchases/:id/verify", func(c *fiber.Ctx) error { c.Locals("user_id", user); return c.Next() },
		NewCoinHandler(coinVerifyDownStub{}).VerifyPurchase)
	resp, err := app.Test(httptest.NewRequest("POST", "/coins/purchases/"+uuid.NewString()+"/verify", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != fiber.StatusServiceUnavailable || body["code"] != "ERR_PAYMENT_PROVIDER_UNAVAILABLE" {
		t.Fatalf("status=%d body=%v, muốn 503 ERR_PAYMENT_PROVIDER_UNAVAILABLE", resp.StatusCode, body)
	}
}
