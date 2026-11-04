package service

// Q7 (QA3) — GET /vouchers/me phải báo mỗi mã có dùng được ngay không (eligible/ineligible_reason), bằng ĐÚNG các luật
// của đường áp mã. Test parity: với mỗi luật, danh sách và ValidateAndApplyVoucher kết luận cùng một lỗi.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

func (f *orderFixture) saveVoucherFor(userID uuid.UUID, v *model.Voucher) {
	f.t.Helper()
	uv := model.UserVoucher{UserID: userID, VoucherID: v.ID, Source: "manual", SavedAt: time.Now()}
	if err := f.db.Create(&uv).Error; err != nil {
		f.t.Fatalf("lưu voucher vào ví: %v", err)
	}
}

func TestGetUserSavedVouchers_EligibilityMatchesApplyRules(t *testing.T) {
	f := newOrderFixture(t)
	vs, _ := newFirstOrderServices(f)
	ctx := context.Background()
	subtotal := decimal.NewFromInt(499000)
	now := time.Now()
	past, future := now.Add(-48*time.Hour), now.Add(48*time.Hour)

	type tc struct {
		name   string
		mutate func(v *model.Voucher)
		prep   func(userID uuid.UUID, v *model.Voucher)
		want   error // nil = dùng được
	}
	cases := []tc{
		{name: "dùng được", want: nil},
		{name: "đơn đầu tiên nhưng đã có đơn", mutate: func(v *model.Voucher) { v.FirstOrderOnly = true },
			prep: func(u uuid.UUID, _ *model.Voucher) { f.priorOrder(u, "completed") }, want: ErrVoucherFirstOrderOnly},
		{name: "đơn đầu tiên và chưa có đơn", mutate: func(v *model.Voucher) { v.FirstOrderOnly = true }, want: nil},
		{name: "hết hạn", mutate: func(v *model.Voucher) { v.EndDate = &past }, want: ErrVoucherExpired},
		{name: "chưa bắt đầu", mutate: func(v *model.Voucher) { v.StartDate = &future }, want: ErrVoucherNotStarted},
		{name: "bị tắt", mutate: func(v *model.Voucher) { v.IsActive = false }, want: ErrVoucherInactive},
		{name: "hết lượt toàn hệ thống", mutate: func(v *model.Voucher) { v.UsageLimit, v.UsedCount = 5, 5 }, want: ErrVoucherUsageLimitExceeded},
		{name: "đổi bằng điểm", mutate: func(v *model.Voucher) { v.DiscountUnit = model.DiscountUnitPoint }, want: ErrVoucherNotMoneyUnit},
		{name: "hết lượt của user", mutate: func(v *model.Voucher) { v.UsagePerUser = 1 },
			prep: func(u uuid.UUID, v *model.Voucher) {
				log := model.VoucherLog{UserID: u, VoucherID: v.ID, VoucherCode: v.Code, OrderID: uuid.New(), Action: "used", Amount: 1}
				if err := f.db.Create(&log).Error; err != nil {
					t.Fatalf("tạo voucher_log: %v", err)
				}
			}, want: ErrVoucherPerUserLimitExceeded},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := firstOrderVoucher(f, false)
			if c.mutate != nil {
				c.mutate(v)
				// IsActive=false bị gorm thay bằng default:true khi Create, nên ghi lại bằng Save.
				if err := f.db.Save(v).Error; err != nil {
					t.Fatalf("cập nhật voucher: %v", err)
				}
			}
			u := f.user()
			if c.prep != nil {
				c.prep(u, v)
			}
			f.saveVoucherFor(u, v)

			views, total, err := vs.GetUserSavedVouchers(ctx, u, 20, 0)
			if err != nil || total != 1 || len(views) != 1 {
				t.Fatalf("list: total=%d len=%d err=%v", total, len(views), err)
			}
			got := views[0]

			if c.want == nil {
				if !got.Eligible || got.IneligibleReason != "" {
					t.Fatalf("muốn dùng được, nhận eligible=%v reason=%q", got.Eligible, got.IneligibleReason)
				}
				return
			}
			wantReason, ok := voucherIneligibleReason(c.want)
			if !ok {
				t.Fatalf("test lỗi: %v chưa có lời giải thích tiếng Việt", c.want)
			}
			if got.Eligible || got.IneligibleReason != wantReason {
				t.Fatalf("eligible=%v reason=%q, muốn false/%q", got.Eligible, got.IneligibleReason, wantReason)
			}
			// Parity: đường áp mã cũng phải từ chối bằng đúng lỗi này (per-user nằm ở bước khoá trong transaction nên
			// kiểm riêng qua LockAndCheckUsagePerUser).
			if errors.Is(c.want, ErrVoucherPerUserLimitExceeded) {
				err = vs.LockAndCheckUsagePerUser(ctx, f.db, v, u)
			} else {
				_, _, err = vs.ValidateAndApplyVoucher(ctx, v.Code, u, subtotal, "")
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("đường áp mã trả %v, muốn %v", err, c.want)
			}
		})
	}
}

// Voucher đã bị xoá mềm: Preload cho Voucher = nil, mục ví phải báo không dùng được thay vì panic.
func TestGetUserSavedVouchers_DeletedVoucherIsIneligible(t *testing.T) {
	f := newOrderFixture(t)
	vs, _ := newFirstOrderServices(f)
	v := firstOrderVoucher(f, false)
	u := f.user()
	f.saveVoucherFor(u, v)
	if err := f.db.Delete(v).Error; err != nil {
		t.Fatalf("xoá voucher: %v", err)
	}
	views, _, err := vs.GetUserSavedVouchers(context.Background(), u, 20, 0)
	if err != nil || len(views) != 1 {
		t.Fatalf("len=%d err=%v", len(views), err)
	}
	if views[0].Eligible || views[0].IneligibleReason == "" {
		t.Fatalf("voucher đã xoá phải ineligible kèm lý do, nhận %+v", views[0])
	}
}
