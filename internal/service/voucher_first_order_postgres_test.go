package service

// QA2 F-02 — voucher "đơn hàng đầu tiên" (cờ dữ liệu first_order_only) trên Postgres thật.
// Quy tắc: user đã có đơn THÀNH CÔNG (completed/processing/refunded) thì mã bị từ chối ở MỌI đường vào
// (tra mã, validate, đặt đơn); đơn bị huỷ/hết hạn/thất bại/đang chờ không tính là "đã mua".

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func firstOrderVoucher(f *orderFixture, firstOnly bool) *model.Voucher {
	f.t.Helper()
	amount := decimal.NewFromInt(50000)
	v := &model.Voucher{
		Code: "F02-" + uuid.NewString()[:8], Name: "F02 " + uuid.NewString()[:6],
		DiscountUnit: model.DiscountUnitMoney, DiscountMethod: model.DiscountMethodFixed,
		DiscountAmountMoney: &amount, AcceptAllPaymentMethods: true, UsagePerUser: 0, FirstOrderOnly: firstOnly,
	}
	if err := f.db.Create(v).Error; err != nil {
		f.t.Fatalf("tạo voucher: %v", err)
	}
	return v
}

func (f *orderFixture) priorOrder(userID uuid.UUID, status string) {
	f.t.Helper()
	o := model.Order{UserID: userID, OrderNumber: "F02-" + uuid.NewString()[:10], Subtotal: decimal.NewFromInt(100000),
		TotalAmount: decimal.NewFromInt(100000), Status: status}
	if err := f.db.Create(&o).Error; err != nil {
		f.t.Fatalf("tạo đơn %s: %v", status, err)
	}
}

func newFirstOrderServices(f *orderFixture) (*VoucherService, *OrderService) {
	vs := NewVoucherService(repository.NewVoucherRepository(f.db), repository.NewUserRepository(f.db))
	os := NewOrderService(repository.NewOrderRepository(f.db), repository.NewOrderItemRepository(f.db),
		repository.NewCourseRepository(f.db), repository.NewCartItemRepository(f.db), nil, vs)
	return vs, os
}

func TestFirstOrderVoucher_ValidateRejectsUserWithSuccessfulOrder(t *testing.T) {
	f := newOrderFixture(t)
	vs, _ := newFirstOrderServices(f)
	ctx := context.Background()
	subtotal := decimal.NewFromInt(499000)
	v := firstOrderVoucher(f, true)

	fresh := f.user()
	if _, d, err := vs.ValidateAndApplyVoucher(ctx, v.Code, fresh, subtotal, ""); err != nil || !d.Equal(decimal.NewFromInt(50000)) {
		t.Fatalf("user mới: discount=%s err=%v, muốn áp dụng", d, err)
	}

	for _, status := range []string{"completed", "processing", "refunded"} {
		u := f.user()
		f.priorOrder(u, status)
		if _, _, err := vs.ValidateAndApplyVoucher(ctx, v.Code, u, subtotal, ""); !errors.Is(err, ErrVoucherFirstOrderOnly) {
			t.Errorf("user có đơn %s: err=%v, muốn ErrVoucherFirstOrderOnly", status, err)
		}
	}

	for _, status := range []string{"pending", "cancelled", "expired", "failed"} {
		u := f.user()
		f.priorOrder(u, status)
		if _, _, err := vs.ValidateAndApplyVoucher(ctx, v.Code, u, subtotal, ""); err != nil {
			t.Errorf("user chỉ có đơn %s (chưa mua được gì): err=%v, muốn vẫn áp dụng được", status, err)
		}
	}
}

func TestFirstOrderVoucher_FlagOffStaysUsableForReturningCustomer(t *testing.T) {
	f := newOrderFixture(t)
	vs, _ := newFirstOrderServices(f)
	v := firstOrderVoucher(f, false)
	u := f.user()
	f.priorOrder(u, "completed")
	if _, _, err := vs.ValidateAndApplyVoucher(context.Background(), v.Code, u, decimal.NewFromInt(499000), ""); err != nil {
		t.Fatalf("voucher không bật first_order_only: err=%v, muốn áp dụng", err)
	}
}

func TestFirstOrderVoucher_CreateOrderRejectsAndLeavesNoTrace(t *testing.T) {
	f := newOrderFixture(t)
	_, os := newFirstOrderServices(f)
	ctx := context.Background()
	v := firstOrderVoucher(f, true)
	course := f.course("F02 course")

	returning := f.user()
	f.priorOrder(returning, "completed")
	req := buyNow(course)
	req.CouponCode = v.Code
	_, err := os.CreateOrder(ctx, returning, req)
	if !errors.Is(err, ErrVoucherFirstOrderOnly) {
		t.Fatalf("CreateOrder với user đã mua: err=%v, muốn ErrVoucherFirstOrderOnly", err)
	}
	var used int32
	f.db.Model(&model.Voucher{}).Where("id = ?", v.ID).Select("used_count").Scan(&used)
	var withVoucher int64
	f.db.Model(&model.Order{}).Where("voucher_id = ?", v.ID).Count(&withVoucher)
	if used != 0 || withVoucher != 0 {
		t.Fatalf("đơn bị từ chối vẫn để lại dấu vết: used_count=%d, đơn dùng voucher=%d", used, withVoucher)
	}

	fresh := f.user()
	req2 := buyNow(course)
	req2.CouponCode = v.Code
	resp, err := os.CreateOrder(ctx, fresh, req2)
	if err != nil || resp == nil {
		t.Fatalf("CreateOrder user mới: err=%v, muốn tạo được", err)
	}
	if !resp.DiscountAmount.Equal(decimal.NewFromInt(50000)) {
		t.Fatalf("user mới: giảm %s, muốn 50000", resp.DiscountAmount)
	}
}

// Đường tra mã công khai (GET /vouchers/code/:code): web tự tính giảm từ kết quả này, nên người xem đã
// đăng nhập mà không còn đủ điều kiện phải nhận lỗi riêng; khách (uuid.Nil) vẫn thấy mã.
func TestFirstOrderVoucher_LookupByCodeForViewer(t *testing.T) {
	f := newOrderFixture(t)
	vs, _ := newFirstOrderServices(f)
	ctx := context.Background()
	v := firstOrderVoucher(f, true)

	returning := f.user()
	f.priorOrder(returning, "completed")
	if _, err := vs.GetVoucherByCodeForViewer(ctx, v.Code, returning); !errors.Is(err, ErrVoucherFirstOrderOnly) {
		t.Fatalf("người xem đã mua: err=%v, muốn ErrVoucherFirstOrderOnly", err)
	}
	if got, err := vs.GetVoucherByCodeForViewer(ctx, v.Code, f.user()); err != nil || got == nil || !got.FirstOrderOnly {
		t.Fatalf("người xem mới: got=%v err=%v, muốn trả voucher kèm first_order_only", got, err)
	}
	if _, err := vs.GetVoucherByCodeForViewer(ctx, v.Code, uuid.Nil); err != nil {
		t.Fatalf("khách: err=%v, muốn xem được mã", err)
	}
}
