package handler_test

// QA7 backlog (#155) mục 1: trường tiền voucher nhận số mũ khổng lồ phải 400 NHANH, không đốt CPU và không chạm service.
// Trường tiền voucher là *float64: encoding/json từ chối 1e20000000 ("out of range") trước khi tới bước so sánh decimal, và
// 1e300 (hợp lệ với float64) bị voucherWholeMoneyErrors chặn bằng so sánh float. Test này ghim cả hai đường để một lần đổi kiểu
// trường sang decimal.Decimal mà quên utils.DecimalExponentWithinBounds sẽ đỏ.

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestQA7B_Voucher_TienSoMuKhongLo_400NhanhKhongChamService(t *testing.T) {
	for _, field := range []string{"discount_amount_money", "max_discount_money", "min_purchase_money"} {
		for _, num := range []string{"1e20000000", "1e300"} {
			svc := &qa7VoucherSvc{}
			app := qa7VoucherApp(svc)
			body := `{"code":"QA7B","name":"Voucher QA7B","discount_unit":"MONEY","discount_method":"FIXED","discount_amount_money":1000,"` + field + `":` + num + `}`
			start := time.Now()
			code, out, raw := qa7Do(t, app, "POST", "/v", body)
			ucode, uout, uraw := qa7Do(t, app, "PUT", "/v/"+uuid.NewString(), `{"`+field+`":`+num+`}`)
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("%s=%s: 2 request mất %v, muốn chặn ngay", field, num, elapsed)
			}
			if code != 400 || out["code"] == nil || svc.calls != 0 {
				t.Fatalf("%s=%s tạo: status=%d body=%s calls=%d, muốn 400 và không gọi service", field, num, code, raw, svc.calls)
			}
			if ucode != 400 || uout["code"] == nil || svc.calls != 0 {
				t.Fatalf("%s=%s sửa: status=%d body=%s calls=%d, muốn 400 và không gọi service", field, num, ucode, uraw, svc.calls)
			}
		}
	}
}

// Số mũ ÂM khổng lồ tràn xuống 0 khi đọc vào float64: với trường min=0 đó là giá trị hợp lệ (0), nên chỉ đòi xử lý nhanh,
// không đòi 400. Với discount_amount_money (gt=0) thì 0 bị từ chối.
func TestQA7B_Voucher_TienSoMuAmKhongLo_XuLyNhanh(t *testing.T) {
	app := qa7VoucherApp(&qa7VoucherSvc{})
	start := time.Now()
	for _, num := range []string{"1e-20000000", "1e-300"} {
		code, _, raw := qa7Do(t, app, "POST", "/v", `{"code":"QA7B","name":"Voucher QA7B","discount_unit":"MONEY","discount_method":"FIXED","discount_amount_money":`+num+`}`)
		if code != 400 {
			t.Fatalf("discount_amount_money=%s: status=%d body=%s, muốn 400", num, code, raw)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("mất %v, muốn xử lý ngay", elapsed)
	}
}
