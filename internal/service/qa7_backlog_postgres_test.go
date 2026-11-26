package service

// QA7 backlog (#155) ở tầng service (Postgres THẬT, schema tạm): HardDeleteVoucher tìm được voucher ĐÃ xoá mềm và dọn cả dữ liệu
// liên quan; id lạ vẫn là ErrVoucherNotFound và không xoá gì. Mutation đã kiểm: trả lại GetVoucherByID có scope ở HardDeleteVoucher
// thì ca "đã xoá mềm" đỏ; bỏ kiểm RowsAffected ở HardDeleteVoucherCascade thì ca "id lạ" đỏ.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestQA7B_HardDeleteVoucher_TimDuocVoucherDaXoaMem_Postgres(t *testing.T) {
	db := isolatedAPISchema(t)
	vs := NewVoucherService(repository.NewVoucherRepository(db), repository.NewUserRepository(db))
	ctx := context.Background()
	exists := func(id uuid.UUID) bool {
		var n int64
		if err := db.Unscoped().Model(&model.Voucher{}).Where("id = ?", id).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n > 0
	}

	// Voucher ĐANG hoạt động: xoá hẳn được (hành vi cũ giữ nguyên).
	live, err := vs.CreateVoucher(ctx, voucherReq("QA7BL"+strings.ToUpper(uuid.NewString()[:8])))
	if err != nil {
		t.Fatal(err)
	}
	if err := vs.HardDeleteVoucher(ctx, live.ID); err != nil || exists(live.ID) {
		t.Fatalf("xoá hẳn voucher đang hoạt động: err=%v, còn dòng=%v", err, exists(live.ID))
	}

	// Voucher ĐÃ xoá mềm: trước đây ErrVoucherNotFound (lookup có scope không thấy dòng), nay xoá hẳn được.
	soft, err := vs.CreateVoucher(ctx, voucherReq("QA7BS"+strings.ToUpper(uuid.NewString()[:8])))
	if err != nil {
		t.Fatal(err)
	}
	if err := vs.DeleteVoucher(ctx, soft.ID); err != nil {
		t.Fatal(err)
	}
	if !exists(soft.ID) {
		t.Fatal("xoá mềm phải giữ dòng")
	}
	if err := vs.HardDeleteVoucher(ctx, soft.ID); err != nil {
		t.Fatalf("xoá hẳn voucher đã xoá mềm: %v, muốn nil", err)
	}
	if exists(soft.ID) {
		t.Fatal("dòng voucher đã xoá mềm vẫn còn sau xoá hẳn")
	}

	// Id lạ (và voucher đã xoá hẳn rồi): không tìm thấy.
	for _, id := range []uuid.UUID{uuid.New(), soft.ID} {
		if err := vs.HardDeleteVoucher(ctx, id); !errors.Is(err, repository.ErrVoucherNotFound) {
			t.Fatalf("xoá hẳn id %s không tồn tại: err=%v, muốn ErrVoucherNotFound", id, err)
		}
	}
}
