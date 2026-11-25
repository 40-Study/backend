package service

// QA6 review L4: đổi tên voucher sang tên đã có ở voucher KHÁC -> ErrVoucherNameExists (handler: 409 ERR_VOUCHER_NAME_EXISTS),
// giống lúc tạo. Giữ nguyên tên của chính nó, hoặc đổi sang tên chưa dùng, vẫn thành công.
// Mutation đã kiểm: bỏ nhánh kiểm tên trong UpdateVoucher thì ca trùng tên ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

func TestQA6_UpdateVoucher_DoiTenTrung_Postgres(t *testing.T) {
	db := isolatedAPISchema(t)
	vs := NewVoucherService(repository.NewVoucherRepository(db), repository.NewUserRepository(db))
	ctx := context.Background()
	mk := func(code, name string) uuid.UUID {
		req := voucherReq(code)
		req.Name = name
		v, err := vs.CreateVoucher(ctx, req)
		if err != nil {
			t.Fatalf("tạo %s: %v", code, err)
		}
		return v.ID
	}
	a, b := mk("QA6A"+uuid.NewString()[:6], "Tên A "+uuid.NewString()[:6]), mk("QA6B"+uuid.NewString()[:6], "Tên B "+uuid.NewString()[:6])
	va, err := vs.vr.GetVoucherByID(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	vb, err := vs.vr.GetVoucherByID(ctx, b)
	if err != nil {
		t.Fatal(err)
	}

	// B đổi sang tên của A -> trùng.
	if _, err := vs.UpdateVoucher(ctx, b, &dto.UpdateVoucherRequest{Name: &va.Name}); !errors.Is(err, ErrVoucherNameExists) {
		t.Errorf("đổi sang tên đã có: err=%v, muốn ErrVoucherNameExists", err)
	}
	// B giữ nguyên tên của chính nó -> không phải trùng.
	if _, err := vs.UpdateVoucher(ctx, b, &dto.UpdateVoucherRequest{Name: &vb.Name}); err != nil {
		t.Errorf("giữ nguyên tên: %v", err)
	}
	// B đổi sang tên mới -> được.
	fresh := "Tên mới " + uuid.NewString()[:6]
	if got, err := vs.UpdateVoucher(ctx, b, &dto.UpdateVoucherRequest{Name: &fresh}); err != nil || got.Name != fresh {
		t.Errorf("đổi sang tên mới: got=%+v err=%v", got, err)
	}
}
