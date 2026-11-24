package handler

// QA6 review L4: PUT /vouchers/:id đổi sang tên đã có -> 409 ERR_VOUCHER_NAME_EXISTS (không còn 400 ERR_UPDATE_VOUCHER chung).
// Bỏ nhánh errors.Is(ErrVoucherNameExists) khỏi UpdateVoucher thì test ĐỎ (400).

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type qa6VoucherRenameSvc struct {
	service.VoucherServiceInterface
}

func (qa6VoucherRenameSvc) UpdateVoucher(context.Context, uuid.UUID, *dto.UpdateVoucherRequest) (*model.Voucher, error) {
	return nil, service.ErrVoucherNameExists
}

func TestQA6_VoucherUpdate_TrungTen409(t *testing.T) {
	app := fiber.New()
	app.Put("/v/:id", NewVoucherHandler(qa6VoucherRenameSvc{}).UpdateVoucher)
	req := httptest.NewRequest("PUT", "/v/"+uuid.NewString(), strings.NewReader(`{"name":"Tên đã có"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != 409 || body["code"] != "ERR_VOUCHER_NAME_EXISTS" {
		t.Errorf("status=%d body=%v, muốn 409 ERR_VOUCHER_NAME_EXISTS", resp.StatusCode, body)
	}
}
