package service

// L1 — voucher "dành riêng" (holders_only) trên Postgres thật, schema tạm (contestFixture).
// Quy tắc: chỉ người đã được cấp / đã lưu (user_vouchers) áp dụng được; người khác nhận ĐÚNG lỗi
// của mã không tồn tại, ở mọi đường vào (đặt đơn, tra mã, tự lưu), để không dò ra mã có thật.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// holdersVoucher tạo voucher MONEY+FIXED 50.000đ còn dùng được; holdersOnly chọn công khai/dành riêng.
func (f *contestFixture) holdersVoucher(holdersOnly bool, end *time.Time) *model.Voucher {
	f.t.Helper()
	amount := decimal.NewFromInt(50000)
	v := &model.Voucher{
		Code: "L1-HOLD-" + uuid.NewString()[:8], Name: "L1 holders " + uuid.NewString()[:6],
		DiscountUnit: model.DiscountUnitMoney, DiscountMethod: model.DiscountMethodFixed,
		DiscountAmountMoney: &amount, AcceptAllPaymentMethods: true, HoldersOnly: holdersOnly, EndDate: end,
	}
	if err := f.db.Create(v).Error; err != nil {
		f.t.Fatalf("tạo voucher: %v", err)
	}
	return v
}

func (f *contestFixture) grant(vs *VoucherService, user, voucher uuid.UUID) {
	f.t.Helper()
	err := f.db.Transaction(func(tx *gorm.DB) error {
		_, err := vs.GrantVoucherTx(context.Background(), tx, user, voucher, "admin_grant", "")
		return err
	})
	if err != nil {
		f.t.Fatalf("cấp voucher: %v", err)
	}
}

// Người chưa giữ voucher dành riêng nhập mã khi đặt đơn: nhận ĐÚNG lỗi của mã không tồn tại; sau khi
// được cấp thì áp dụng bình thường. Thử cả voucher đã hết hạn để chứng minh thứ tự kiểm: holders_only
// đứng TRƯỚC "hết hạn", nếu không lỗi khác nhau sẽ lộ mã có thật.
func TestValidateAndApplyVoucher_HoldersOnly(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	ctx := context.Background()
	stranger, holder := f.user("student"), f.user("student")
	subtotal := decimal.NewFromInt(200000)

	v := f.holdersVoucher(true, nil)
	past := time.Now().Add(-time.Hour)
	expired := f.holdersVoucher(true, &past)
	f.grant(vs, holder, v.ID)

	_, _, missingErr := vs.ValidateAndApplyVoucher(ctx, "L1-NOPE-"+uuid.NewString()[:8], stranger, subtotal, "")
	if !errors.Is(missingErr, repository.ErrVoucherNotFound) {
		t.Fatalf("mã không tồn tại: muốn repository.ErrVoucherNotFound, nhận %v", missingErr)
	}

	for name, code := range map[string]string{"còn hạn": v.Code, "hết hạn": expired.Code} {
		_, _, err := vs.ValidateAndApplyVoucher(ctx, code, stranger, subtotal, "")
		if err != missingErr {
			t.Errorf("người ngoài + voucher dành riêng %s: nhận %v, muốn y hệt lỗi mã không tồn tại (%v)", name, err, missingErr)
		}
	}

	got, discount, err := vs.ValidateAndApplyVoucher(ctx, v.Code, holder, subtotal, "")
	if err != nil || got == nil || got.ID != v.ID || !discount.Equal(decimal.NewFromInt(50000)) {
		t.Fatalf("người được cấp: voucher=%v discount=%s err=%v, muốn áp dụng giảm 50000", got, discount, err)
	}
}

// Mặc định công khai: voucher không bật holders_only áp dụng cho bất kỳ ai như cũ.
func TestValidateAndApplyVoucher_PublicStaysPublic(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	v := f.holdersVoucher(false, nil)
	_, discount, err := vs.ValidateAndApplyVoucher(context.Background(), v.Code, f.user("student"), decimal.NewFromInt(200000), "")
	if err != nil || !discount.Equal(decimal.NewFromInt(50000)) {
		t.Fatalf("voucher công khai: discount=%s err=%v, muốn áp dụng", discount, err)
	}
}

// Tự lưu không phải cửa hậu: người ngoài POST /vouchers/:id/save một voucher dành riêng nhận lỗi
// không-tồn-tại và KHÔNG có dòng user_vouchers nào được tạo (nếu tạo được thì tự cấp quyền cho mình).
func TestSaveVoucher_HoldersOnlyCannotSelfSave(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	ctx := context.Background()
	stranger, holder := f.user("student"), f.user("student")
	v := f.holdersVoucher(true, nil)
	f.grant(vs, holder, v.ID)

	_, err := vs.SaveVoucher(ctx, stranger, &dto.SaveVoucherRequest{VoucherCode: v.Code})
	if !errors.Is(err, repository.ErrVoucherNotFound) {
		t.Fatalf("người ngoài tự lưu: nhận %v, muốn ErrVoucherNotFound", err)
	}
	if n := f.count("user_vouchers", "user_id = ?", stranger); n != 0 {
		t.Fatalf("người ngoài tự lưu bị chặn mà vẫn có %d user_vouchers", n)
	}
	if _, err := vs.SaveVoucher(ctx, holder, &dto.SaveVoucherRequest{VoucherCode: v.Code}); err != nil {
		t.Fatalf("người đã giữ lưu lại voucher: %v", err)
	}
	// Voucher công khai vẫn tự lưu được.
	pub := f.holdersVoucher(false, nil)
	if _, err := vs.SaveVoucher(ctx, stranger, &dto.SaveVoucherRequest{VoucherCode: pub.Code}); err != nil {
		t.Fatalf("tự lưu voucher công khai: %v", err)
	}
}

// Tra mã công khai: khách và người ngoài nhận lỗi như mã không tồn tại; người giữ thấy được.
func TestGetVoucherByCodeForViewer_HoldersOnly(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	ctx := context.Background()
	stranger, holder := f.user("student"), f.user("student")
	v := f.holdersVoucher(true, nil)
	pub := f.holdersVoucher(false, nil)
	f.grant(vs, holder, v.ID)

	for name, viewer := range map[string]uuid.UUID{"khách": uuid.Nil, "người ngoài": stranger} {
		if _, err := vs.GetVoucherByCodeForViewer(ctx, v.Code, viewer); !errors.Is(err, repository.ErrVoucherNotFound) {
			t.Errorf("%s tra voucher dành riêng: nhận %v, muốn ErrVoucherNotFound", name, err)
		}
	}
	if got, err := vs.GetVoucherByCodeForViewer(ctx, v.Code, holder); err != nil || got.ID != v.ID {
		t.Errorf("người giữ tra voucher: got=%v err=%v", got, err)
	}
	if got, err := vs.GetVoucherByCodeForViewer(ctx, pub.Code, uuid.Nil); err != nil || got.ID != pub.ID {
		t.Errorf("khách tra voucher công khai: got=%v err=%v", got, err)
	}
}

// Danh sách công khai không bao giờ có voucher dành riêng, kể cả đang còn hạn và bật.
func TestGetPublicVouchers_ExcludesHoldersOnly(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	hidden := f.holdersVoucher(true, nil)
	shown := f.holdersVoucher(false, nil)

	list, _, err := vs.GetPublicVouchers(context.Background(), 100, 0)
	if err != nil {
		t.Fatalf("GetPublicVouchers: %v", err)
	}
	var seenShown bool
	for _, v := range list {
		if v.ID == hidden.ID {
			t.Fatalf("voucher dành riêng %s lộ trong danh sách công khai", hidden.Code)
		}
		seenShown = seenShown || v.ID == shown.ID
	}
	if !seenShown {
		t.Fatalf("voucher công khai %s mất khỏi danh sách", shown.Code)
	}
}

// Admin đặt holders_only lúc tạo và đổi lúc sửa; bỏ trống = công khai (mặc định). Cột not null default false.
func TestVoucherAdmin_HoldersOnlyCreateUpdateDefault(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	ctx := context.Background()
	amount := 10000.0
	yes := true
	mk := func(holders *bool) *model.Voucher {
		v, err := vs.CreateVoucher(ctx, &dto.CreateVoucherRequest{
			Code: "L1-ADM-" + uuid.NewString()[:8], Name: "L1 admin " + uuid.NewString()[:6],
			DiscountUnit: "MONEY", DiscountMethod: "FIXED", DiscountAmountMoney: &amount,
			AcceptAllPaymentMethods: true, HoldersOnly: holders,
		})
		if err != nil {
			t.Fatalf("CreateVoucher: %v", err)
		}
		return v
	}

	if v := mk(nil); v.HoldersOnly {
		t.Errorf("không gửi holders_only: muốn công khai (false)")
	}
	created := mk(&yes)
	if !created.HoldersOnly {
		t.Fatalf("tạo với holders_only=true mà trả false")
	}
	var stored model.Voucher
	f.db.First(&stored, "id = ?", created.ID)
	if !stored.HoldersOnly {
		t.Fatalf("holders_only không được lưu vào DB")
	}

	no := false
	if v, err := vs.UpdateVoucher(ctx, created.ID, &dto.UpdateVoucherRequest{HoldersOnly: &no}); err != nil || v.HoldersOnly {
		t.Fatalf("sửa holders_only=false: v=%v err=%v", v, err)
	}
	if v, err := vs.UpdateVoucher(ctx, created.ID, &dto.UpdateVoucherRequest{}); err != nil || v.HoldersOnly {
		t.Fatalf("sửa không gửi holders_only phải giữ nguyên (false): v=%v err=%v", v, err)
	}
	if v, err := vs.UpdateVoucher(ctx, created.ID, &dto.UpdateVoucherRequest{HoldersOnly: &yes}); err != nil || !v.HoldersOnly {
		t.Fatalf("sửa holders_only=true: v=%v err=%v", v, err)
	}
	if v, err := vs.UpdateVoucher(ctx, created.ID, &dto.UpdateVoucherRequest{}); err != nil || !v.HoldersOnly {
		t.Fatalf("sửa không gửi holders_only phải giữ nguyên (true): v=%v err=%v", v, err)
	}
}

// Migration: cột có, NOT NULL, mặc định false; hàng chèn không nhắc tới cột này vẫn là công khai.
func TestVoucherHoldersOnlyColumn_NotNullDefaultFalse(t *testing.T) {
	f := newContestFixture(t)
	var col struct {
		IsNullable    string
		ColumnDefault string
	}
	err := f.db.Raw(`SELECT is_nullable, column_default FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'vouchers' AND column_name = 'holders_only'`).Scan(&col).Error
	if err != nil || col.IsNullable != "NO" || col.ColumnDefault != "false" {
		t.Fatalf("holders_only: nullable=%q default=%q err=%v, muốn NO/false", col.IsNullable, col.ColumnDefault, err)
	}
	if err := f.db.Exec(`INSERT INTO vouchers (id, code, name, discount_unit, discount_method) VALUES (gen_random_uuid(), 'L1-RAW', 'L1 raw', 'MONEY', 'FIXED')`).Error; err != nil {
		t.Fatalf("chèn voucher thô: %v", err)
	}
	var holders bool
	f.db.Raw(`SELECT holders_only FROM vouchers WHERE code = 'L1-RAW'`).Scan(&holders)
	if holders {
		t.Fatalf("voucher chèn không nhắc holders_only phải công khai")
	}
}
