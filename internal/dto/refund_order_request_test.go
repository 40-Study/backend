package dto

import (
	"testing"

	"study.com/v1/internal/utils"
)

// Quyet dinh chu du an #1 (27/09/2026): hoan tien = admin xac nhan DA chuyen khoan thu cong
// NGOAI he thong — KHONG hoan vao vi xu. Test nay khoa cung "oneof" tren RefundMethod: neu ai do
// vo tinh them lai "wallet_credit" vao tag validate, test PHAI do.
func TestRefundOrderRequest_RejectsWalletCredit(t *testing.T) {
	req := RefundOrderRequest{
		Reason:       "hoc vien khieu nai noi dung sai",
		RefundMethod: "wallet_credit",
	}

	errs := utils.ValidateStruct(req)

	if len(errs) == 0 {
		t.Fatalf("expected validation to reject refund_method=wallet_credit, got no errors (validate tag da bi noi long lai)")
	}

	found := false
	for _, e := range errs {
		if e.Field == "refund_method" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a refund_method validation error, got: %+v", errs)
	}
}

func TestRefundOrderRequest_AcceptsManualBankTransfer(t *testing.T) {
	req := RefundOrderRequest{
		Reason:       "hoc vien khieu nai noi dung sai",
		RefundMethod: "manual_bank_transfer",
	}

	errs := utils.ValidateStruct(req)

	for _, e := range errs {
		if e.Field == "refund_method" {
			t.Fatalf("expected refund_method=manual_bank_transfer to be valid, got error: %+v", e)
		}
	}
}

func TestRefundOrderRequest_RejectsEmptyRefundMethod(t *testing.T) {
	req := RefundOrderRequest{
		Reason:       "hoc vien khieu nai noi dung sai",
		RefundMethod: "",
	}

	errs := utils.ValidateStruct(req)

	found := false
	for _, e := range errs {
		if e.Field == "refund_method" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected refund_method=\"\" (missing) to fail required validation, got: %+v", errs)
	}
}
