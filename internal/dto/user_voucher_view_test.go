package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// Hợp đồng JSON của GET /vouchers/me: các field cũ của user_voucher giữ nguyên ở cấp ngoài, thêm eligible và
// ineligible_reason (luôn có mặt, kể cả khi dùng được).
func TestUserVoucherView_JSONContract(t *testing.T) {
	view := dto.UserVoucherView{
		UserVoucher:      &model.UserVoucher{ID: uuid.New(), VoucherID: uuid.New(), Voucher: &model.Voucher{Code: "WELCOME20"}},
		Eligible:         false,
		IneligibleReason: "x",
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "voucher_id", "voucher", "eligible", "ineligible_reason"} {
		if _, ok := m[k]; !ok {
			t.Errorf("thiếu key %q trong %s", k, raw)
		}
	}
	if m["eligible"] != false || m["ineligible_reason"] != "x" {
		t.Errorf("eligible/ineligible_reason sai: %s", raw)
	}
	ok, _ := json.Marshal(dto.UserVoucherView{UserVoucher: &model.UserVoucher{}, Eligible: true})
	var m2 map[string]any
	_ = json.Unmarshal(ok, &m2)
	if v, present := m2["ineligible_reason"]; !present || v != "" {
		t.Errorf("ineligible_reason phải là chuỗi rỗng khi dùng được: %s", ok)
	}
}
