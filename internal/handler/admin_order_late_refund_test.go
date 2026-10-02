package handler

// Lane P: POST /orders/admin/:id/late-refund — ánh xạ lỗi service sang mã HTTP và body tuỳ chọn.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type fakeAdminOrderLateRefund struct {
	service.AdminOrderServiceInterface
	resp      *dto.LateRefundResponse
	err       error
	gotNote   string
	gotRef    string
	gotRefs   []string
	callCount int
}

func (f *fakeAdminOrderLateRefund) MarkLatePaymentRefunded(_ context.Context, _, _ uuid.UUID, note, ref string, refs []string) (*dto.LateRefundResponse, error) {
	f.callCount++
	f.gotNote, f.gotRef, f.gotRefs = note, ref, refs
	return f.resp, f.err
}

func callLateRefund(t *testing.T, svc service.AdminOrderServiceInterface, id, body string) (int, map[string]interface{}) {
	t.Helper()
	h := NewAdminOrderHandler(nil, svc, nil)
	app := fiber.New()
	app.Post("/orders/admin/:id/late-refund", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return h.MarkLatePaymentRefunded(c)
	})
	req := httptest.NewRequest("POST", "/orders/admin/"+id+"/late-refund", strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestMarkLatePaymentRefunded_EmptyBodyIsAllowed(t *testing.T) {
	f := &fakeAdminOrderLateRefund{resp: &dto.LateRefundResponse{ID: uuid.New(), LateRefundedAt: time.Now()}}
	status, _ := callLateRefund(t, f, uuid.NewString(), "")
	if status != fiber.StatusOK || f.callCount != 1 {
		t.Fatalf("body rỗng: status=%d calls=%d, muốn 200 và gọi service 1 lần", status, f.callCount)
	}
}

func TestMarkLatePaymentRefunded_PassesTrimmedNoteAndRef(t *testing.T) {
	f := &fakeAdminOrderLateRefund{resp: &dto.LateRefundResponse{ID: uuid.New(), LateRefundedAt: time.Now()}}
	status, _ := callLateRefund(t, f, uuid.NewString(), `{"note":"  đã CK  ","transaction_ref":" FT-9 "}`)
	if status != fiber.StatusOK || f.gotNote != "đã CK" || f.gotRef != "FT-9" {
		t.Fatalf("status=%d note=%q ref=%q, muốn 200 và giá trị đã trim", status, f.gotNote, f.gotRef)
	}
}

func TestMarkLatePaymentRefunded_ErrorMapping(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		code   string
	}{
		"không có cờ tiền về muộn": {service.ErrLateRefundNotNeeded, fiber.StatusBadRequest, "refund_not_needed"},
		"đơn không tồn tại":        {service.ErrOrderNotFound, fiber.StatusNotFound, "not_found"},
		// Service bọc lỗi kèm mã lạ (fmt.Errorf("%w: %q")): handler phải nhận ra bằng errors.Is, trả 400 chứ không 500.
		"mã giao dịch hoàn lạ": {fmt.Errorf("%w: %q", service.ErrLateRefundUnknownRef, "FT-LA"), fiber.StatusBadRequest, "unknown_late_payment"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := callLateRefund(t, &fakeAdminOrderLateRefund{err: c.err}, uuid.NewString(), "")
			if status != c.status || body["error"] != c.code {
				t.Fatalf("status=%d body=%v, muốn %d error=%s", status, body, c.status, c.code)
			}
		})
	}
}

func TestMarkLatePaymentRefunded_RejectsBadInput(t *testing.T) {
	f := &fakeAdminOrderLateRefund{}
	if status, _ := callLateRefund(t, f, "khong-phai-uuid", ""); status != fiber.StatusBadRequest {
		t.Fatalf("id sai trả %d, muốn 400", status)
	}
	long := strings.Repeat("a", 501)
	if status, _ := callLateRefund(t, f, uuid.NewString(), `{"note":"`+long+`"}`); status != fiber.StatusBadRequest {
		t.Fatalf("ghi chú quá dài trả %d, muốn 400", status)
	}
	if f.callCount != 0 {
		t.Fatalf("đầu vào sai vẫn gọi xuống service %d lần, muốn 0", f.callCount)
	}
}
