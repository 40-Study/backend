package service

// L6 mục 6 và 8 trên Postgres thật, schema tạm (contestFixture):
//   - voucher holders_only: người giữ KHÔNG tự bỏ lưu được (quyền dùng nằm ở dòng user_vouchers);
//     voucher công khai vẫn bỏ lưu bình thường;
//   - sửa voucher: null xoá được ngày bắt đầu/kết thúc và trần giảm;
//   - danh sách admin: phân trang thật, không lặp/nhảy dòng giữa các trang.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func caseTestUnsaveVoucher_HoldersOnlyIsNotRemovable(t *testing.T, f *contestFixture) {
	vs := newTestVoucherService(f)
	ctx := context.Background()
	holder := f.user("student")
	private := f.holdersVoucher(true, nil)
	f.grant(vs, holder, private.ID)

	err := vs.UnsaveVoucher(ctx, holder, private.ID)
	if !errors.Is(err, ErrHoldersOnlyVoucherNotRemovable) {
		t.Fatalf("bỏ lưu voucher dành riêng: nhận %v, muốn ErrHoldersOnlyVoucherNotRemovable", err)
	}
	if n := f.count("user_vouchers", "user_id = ? AND voucher_id = ? AND deleted_at IS NULL", holder, private.ID); n != 1 {
		t.Fatalf("quyền giữ voucher đã mất sau khi bị từ chối bỏ lưu: còn %d dòng, muốn 1", n)
	}
	// Vẫn dùng được sau lần bấm nhầm.
	if _, err := vs.GetVoucherByCodeForViewer(ctx, private.Code, holder); err != nil {
		t.Fatalf("người giữ mất quyền tra voucher sau khi bị từ chối bỏ lưu: %v", err)
	}

	// Người KHÔNG giữ voucher dành riêng: cùng lỗi với voucher không tồn tại (không xác nhận UUID là voucher dành
	// riêng), và không đụng tới quyền của người giữ thật.
	stranger := f.user("student")
	if err := vs.UnsaveVoucher(ctx, stranger, private.ID); !errors.Is(err, ErrUserVoucherNotFound) {
		t.Fatalf("người lạ bỏ lưu voucher dành riêng: nhận %v, muốn ErrUserVoucherNotFound (không lộ voucher)", err)
	}
	if err := vs.UnsaveVoucher(ctx, stranger, uuid.New()); !errors.Is(err, ErrUserVoucherNotFound) {
		t.Fatalf("voucher không tồn tại: nhận %v, muốn cùng ErrUserVoucherNotFound", err)
	}

	// Voucher công khai: tự lưu rồi bỏ lưu bình thường.
	pub := f.holdersVoucher(false, nil)
	if _, err := vs.SaveVoucher(ctx, holder, &dto.SaveVoucherRequest{VoucherCode: pub.Code}); err != nil {
		t.Fatalf("lưu voucher công khai: %v", err)
	}
	if err := vs.UnsaveVoucher(ctx, holder, pub.ID); err != nil {
		t.Fatalf("bỏ lưu voucher công khai: %v", err)
	}
	if n := f.count("user_vouchers", "user_id = ? AND voucher_id = ? AND deleted_at IS NULL", holder, pub.ID); n != 0 {
		t.Fatalf("voucher công khai bỏ lưu xong còn %d dòng, muốn 0", n)
	}
}

func caseTestUpdateVoucher_NullClearsDatesAndCap(t *testing.T, f *contestFixture) {
	vs := newTestVoucherService(f)
	ctx := context.Background()
	start, end := time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour)
	capMoney := decimal.NewFromInt(30000)
	v := f.holdersVoucher(false, &end)
	if err := f.db.Model(&model.Voucher{}).Where("id = ?", v.ID).
		Updates(map[string]any{"start_date": start, "max_discount_money": capMoney, "max_discount_points": 50}).Error; err != nil {
		t.Fatalf("dựng voucher có ngày và trần: %v", err)
	}
	update := func(body string) *model.Voucher {
		var req dto.UpdateVoucherRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatalf("unmarshal %s: %v", body, err)
		}
		got, err := vs.UpdateVoucher(ctx, v.ID, &req)
		if err != nil {
			t.Fatalf("UpdateVoucher %s: %v", body, err)
		}
		return got
	}

	// Không đụng tới các trường này thì giữ nguyên.
	if got := update(`{"name":"Voucher đổi tên"}`); got.StartDate == nil || got.EndDate == nil || got.MaxDiscountMoney == nil || got.MaxDiscountPoints != 50 {
		t.Fatalf("sửa tên làm mất ngày/trần: %+v", got)
	}
	got := update(`{"start_date":null,"end_date":null,"max_discount_money":null,"max_discount_points":null}`)
	if got.StartDate != nil || got.EndDate != nil || got.MaxDiscountMoney != nil || got.MaxDiscountPoints != 0 {
		t.Fatalf("null không xoá: start=%v end=%v capMoney=%v capPoints=%d", got.StartDate, got.EndDate, got.MaxDiscountMoney, got.MaxDiscountPoints)
	}
	var stored model.Voucher
	if err := f.db.First(&stored, "id = ?", v.ID).Error; err != nil {
		t.Fatalf("đọc lại: %v", err)
	}
	if stored.StartDate != nil || stored.EndDate != nil || stored.MaxDiscountMoney != nil || stored.MaxDiscountPoints != 0 {
		t.Fatalf("DB vẫn còn giá trị sau khi xoá: %+v", stored)
	}
}

func caseTestGetAllVouchers_RealPagination(t *testing.T, f *contestFixture) {
	vs := newTestVoucherService(f)
	ctx := context.Background()
	const n = 7
	made := map[uuid.UUID]bool{}
	for i := 0; i < n; i++ {
		made[f.holdersVoucher(false, nil).ID] = true
	}
	// Mọi voucher cùng created_at: thứ tự phải ổn định (khoá phụ id) để các trang không chồng/khuyết.
	f.db.Exec("UPDATE vouchers SET created_at = ?", time.Now().Add(-time.Hour))

	seen := map[uuid.UUID]int{}
	for offset := 0; ; offset += 3 {
		items, total, err := vs.GetAllVouchers(ctx, &dto.GetVouchersRequest{Limit: 3, Offset: offset})
		if err != nil {
			t.Fatalf("offset %d: %v", offset, err)
		}
		if total < n {
			t.Fatalf("total_count = %d, muốn >= %d", total, n)
		}
		if len(items) > 3 {
			t.Fatalf("offset %d: %d dòng, muốn tối đa 3", offset, len(items))
		}
		for _, it := range items {
			seen[it.ID]++
		}
		if len(items) == 0 || offset+len(items) >= int(total) {
			break
		}
	}
	for id := range made {
		if seen[id] != 1 {
			t.Errorf("voucher %s xuất hiện %d lần qua các trang, muốn đúng 1", id, seen[id])
		}
	}
}

func TestVoucherL6(t *testing.T) {
	root := newContestFixture(t)
	cases := []struct {
		name string
		fn   func(*testing.T, *contestFixture)
	}{
		{"UnsaveVoucher_HoldersOnlyIsNotRemovable", caseTestUnsaveVoucher_HoldersOnlyIsNotRemovable},
		{"UpdateVoucher_NullClearsDatesAndCap", caseTestUpdateVoucher_NullClearsDatesAndCap},
		{"GetAllVouchers_RealPagination", caseTestGetAllVouchers_RealPagination},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { c.fn(t, root.forT(t)) })
	}
}
