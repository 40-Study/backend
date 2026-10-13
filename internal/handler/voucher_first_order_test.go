package handler

// QA2 F-02: tra mã GET /vouchers/code/:code với voucher "đơn hàng đầu tiên" mà người xem đã mua rồi
// phải trả 409 VOUCHER_FIRST_ORDER_ONLY (không phải 404 "không tồn tại") để web báo đúng lý do.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type firstOrderVoucherStub struct {
	service.VoucherServiceInterface
}

func (s *firstOrderVoucherStub) GetVoucherByCodeForViewer(context.Context, string, uuid.UUID) (*model.Voucher, error) {
	return nil, service.ErrVoucherFirstOrderOnly
}

func TestGetVoucherByCode_FirstOrderOnlyReturns409WithCode(t *testing.T) {
	user := uuid.New()
	status, body := doVoucher(t, NewVoucherHandler(&firstOrderVoucherStub{}), &user, "GET", "/vouchers/code/WELCOME20")
	if status != fiber.StatusConflict {
		t.Fatalf("status = %d (%s), muốn 409", status, body)
	}
	var out map[string]string
	_ = json.Unmarshal([]byte(body), &out)
	if out["code"] != "VOUCHER_FIRST_ORDER_ONLY" || out["message"] == "" {
		t.Fatalf("body = %s, muốn code VOUCHER_FIRST_ORDER_ONLY kèm message", body)
	}
}
