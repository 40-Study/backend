package service

// QA3-11: admin tạo được voucher mã "qa3 test!" (có khoảng trắng và dấu chấm than) vì validation chỉ kiểm độ dài.
// Mã phải khớp ^[A-Z0-9_-]+$ sau khi chuẩn hoá in hoa; mã sai là ErrVoucherCodeInvalid (tiếng Việt) và KHÔNG tạo dòng nào.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func voucherReq(code string) *dto.CreateVoucherRequest {
	amount := 10000.0
	return &dto.CreateVoucherRequest{
		Code: code, Name: "QA3 voucher " + uuid.NewString()[:8],
		DiscountUnit: "MONEY", DiscountMethod: "FIXED", DiscountAmountMoney: &amount, AcceptAllPaymentMethods: true,
	}
}

func TestCreateVoucher_CodeFormat(t *testing.T) {
	db := isolatedAPISchema(t)
	vs := NewVoucherService(repository.NewVoucherRepository(db), repository.NewUserRepository(db))
	ctx := context.Background()

	for _, bad := range []string{"qa3 test!", "AB CD", "ABC!", "mã-việt", "A.B.C", "ABC\nDEF", "!!!"} {
		if _, err := vs.CreateVoucher(ctx, voucherReq(bad)); !errors.Is(err, ErrVoucherCodeInvalid) {
			t.Errorf("mã %q: err = %v, muốn ErrVoucherCodeInvalid", bad, err)
		}
	}
	var n int64
	db.Model(&model.Voucher{}).Count(&n)
	if n != 0 {
		t.Fatalf("mã sai vẫn tạo %d voucher", n)
	}

	// Mã hợp lệ viết thường/có khoảng trắng hai đầu được chuẩn hoá thành IN HOA.
	for in, want := range map[string]string{"  summer-50_k ": "SUMMER-50_K", "welcome20": "WELCOME20", "VIP_100": "VIP_100"} {
		v, err := vs.CreateVoucher(ctx, voucherReq(in))
		if err != nil {
			t.Fatalf("mã %q hợp lệ nhưng bị từ chối: %v", in, err)
		}
		if v.Code != want {
			t.Errorf("mã %q lưu thành %q, muốn %q", in, v.Code, want)
		}
	}

	// Trùng mã sau chuẩn hoá: "welcome20" đã có thì "WELCOME20" bị từ chối.
	if _, err := vs.CreateVoucher(ctx, voucherReq("Welcome20")); err == nil {
		t.Error("trùng mã sau chuẩn hoá in hoa phải bị từ chối")
	}
}
