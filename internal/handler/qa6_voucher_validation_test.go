package handler

// QA6-A05 (C9): tạo/sửa voucher: mỗi lỗi 400 nêu đúng trường và lý do bằng tiếng Việt; trùng mã -> 409 ERR_VOUCHER_CODE_EXISTS.
// Bỏ voucherInvalidBody/voucherWholeMoneyErrors khỏi handler thì các ca tràn int32, số lẻ và tiền lẻ ĐỎ (lại là câu chung).

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

type qa6VoucherSvc struct {
	service.VoucherServiceInterface
	createErr error
}

func (s qa6VoucherSvc) CreateVoucher(context.Context, *dto.CreateVoucherRequest) (*model.Voucher, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &model.Voucher{ID: uuid.New()}, nil
}

func (s qa6VoucherSvc) UpdateVoucher(context.Context, uuid.UUID, *dto.UpdateVoucherRequest) (*model.Voucher, error) {
	return &model.Voucher{ID: uuid.New()}, nil
}

func TestQA6_VoucherCreateUpdate_ThongBaoTungTruong(t *testing.T) {
	base := func(extra string) string {
		return `{"code":"QA6TEST","name":"Voucher thử","discount_unit":"MONEY","discount_method":"FIXED","discount_amount_money":1000` + extra + `}`
	}
	longName := strings.Repeat("a", 300)
	for _, tc := range []struct {
		name, method, path, body string
		createErr                error
		status                   int
		code, msgContains        string
	}{
		{"hợp lệ", "POST", "/v", base(""), nil, 201, "", ""},
		{"usage_limit tràn int32", "POST", "/v", base(`,"usage_limit":3000000000`), nil, 400, "ERR_VALIDATION", "Tổng số lượt dùng"},
		{"điểm giảm số lẻ", "POST", "/v", `{"code":"QA6PTS","name":"Voucher điểm","discount_unit":"POINT","discount_method":"FIXED","discount_amount_points":1.5}`, nil, 400, "ERR_VALIDATION", "Số điểm giảm"},
		{"tiền giảm số lẻ", "POST", "/v", strings.Replace(base(""), `"discount_amount_money":1000`, `"discount_amount_money":1500.5`, 1), nil, 400, "ERR_VALIDATION", "Số tiền giảm phải là số nguyên"},
		{"đơn tối thiểu số lẻ", "POST", "/v", base(`,"min_purchase_money":99.5`), nil, 400, "ERR_VALIDATION", "Đơn tối thiểu"},
		{"tên 300 ký tự", "POST", "/v", strings.Replace(base(""), "Voucher thử", longName, 1), nil, 400, "ERR_VALIDATION", "Tên voucher phải có từ 3 đến 255"},
		{"phần trăm > 100", "POST", "/v", `{"code":"QA6PCT","name":"Voucher phần trăm","discount_unit":"MONEY","discount_method":"PERCENT","discount_percent":150}`, nil, 400, "ERR_VALIDATION", "Phần trăm giảm"},
		{"trùng mã", "POST", "/v", base(""), service.ErrVoucherCodeExists, 409, "ERR_VOUCHER_CODE_EXISTS", "đã tồn tại"},
		{"sửa: trần điểm tràn int32", "PUT", "/v/" + uuid.NewString(), `{"max_discount_points":3000000000}`, nil, 400, "ERR_VALIDATION", "Số điểm giảm tối đa"},
		{"sửa: tiền lẻ", "PUT", "/v/" + uuid.NewString(), `{"discount_amount_money":10.25}`, nil, 400, "ERR_VALIDATION", "Số tiền giảm phải là số nguyên"},
		{"sửa: hợp lệ", "PUT", "/v/" + uuid.NewString(), `{"discount_amount_money":2000,"usage_limit":10}`, nil, 200, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewVoucherHandler(qa6VoucherSvc{createErr: tc.createErr})
			app := fiber.New()
			app.Post("/v", h.CreateVoucher)
			app.Put("/v/:id", h.UpdateVoucher)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			msg, _ := body["message"].(string)
			if resp.StatusCode != tc.status || (tc.code != "" && body["code"] != tc.code) || !strings.Contains(msg, tc.msgContains) {
				t.Fatalf("status=%d body=%v, muốn %d %s ~%q", resp.StatusCode, body, tc.status, tc.code, tc.msgContains)
			}
		})
	}
}
