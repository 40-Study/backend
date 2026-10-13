package service

// QA2 F-02 / review M1 — luật "đơn đầu tiên" phải được kiểm lại LÚC LẤY TIỀN (pending -> processing trong
// CreatePaymentIntent), không chỉ lúc tạo đơn: mở sẵn đơn B có voucher rồi thanh toán đơn A trước không được lách
// luật; đặt lại đúng giỏ đang mở vẫn nhận lỗi đơn đang mở như trước.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func (f *orderFixture) orderWithVoucher(os *OrderService, user uuid.UUID, code string, courses ...uuid.UUID) (*dto.OrderResponse, error) {
	req := buyNow(courses...)
	req.CouponCode = code
	return os.CreateOrder(context.Background(), user, req)
}

func (f *orderFixture) intent(user, orderID uuid.UUID) error {
	_, err := f.paymentService().CreatePaymentIntent(context.Background(), user, orderID, false, "qr_transfer")
	return err
}

func TestFirstOrderVoucher_PaymentIntentRechecksRule(t *testing.T) {
	cases := []struct {
		name      string
		otherStat string // trạng thái đơn A (không voucher) khi B xin mã thanh toán; "" = A chưa thanh toán
		wantErr   bool
	}{
		{"A đã completed rồi mới thanh toán B", "completed", true},
		{"A đang processing khi B xin mã", "processing", true},
		{"A chỉ pending (chưa thanh toán)", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newOrderFixture(t)
			_, os := newFirstOrderServices(f)
			user := f.user()
			v := firstOrderVoucher(f, true)

			b, err := f.orderWithVoucher(os, user, v.Code, f.course("M1 course B"))
			if err != nil {
				t.Fatalf("tạo đơn B có voucher: %v", err)
			}
			a := f.createOrder(user, f.course("M1 course A"))
			switch tc.otherStat {
			case "completed":
				if err := f.intent(user, a.ID); err != nil {
					t.Fatal(err)
				}
				f.exec("UPDATE orders SET status = 'completed' WHERE id = ?", a.ID)
			case "processing":
				if err := f.intent(user, a.ID); err != nil {
					t.Fatal(err)
				}
			}

			err = f.intent(user, b.ID)
			stored := f.loadOrder(b.ID)
			if tc.wantErr {
				if !errors.Is(err, ErrVoucherFirstOrderOnly) {
					t.Fatalf("CreatePaymentIntent(B): err=%v, muốn ErrVoucherFirstOrderOnly", err)
				}
				if stored.Status != "pending" || stored.PaymentTransactionID != nil {
					t.Fatalf("đơn B bị từ chối vẫn được cấp mã: status=%s code=%v", stored.Status, stored.PaymentTransactionID)
				}
				return
			}
			if err != nil || stored.Status != "processing" {
				t.Fatalf("B là đơn đầu tiên: err=%v status=%s, muốn mở được thanh toán", err, stored.Status)
			}
		})
	}
}

// Voucher không giới hạn mỗi user (usage_per_user=0): đơn #1 đang processing giữ mã thì đơn #2 cho khoá khác
// dùng cùng mã phải bị chặn; đặt lại ĐÚNG giỏ của đơn #1 vẫn nhận ErrOrderInProgress như trước F-02.
func TestFirstOrderVoucher_UnlimitedPerUserSecondOrderBlocked(t *testing.T) {
	f := newOrderFixture(t)
	_, os := newFirstOrderServices(f)
	user := f.user()
	v := firstOrderVoucher(f, true)
	// usage_per_user có default:1 ở DB nên GORM bỏ qua 0 khi Create: đặt tường minh để voucher thật sự không giới hạn.
	f.exec("UPDATE vouchers SET usage_per_user = 0 WHERE id = ?", v.ID)
	courseX, courseY := f.course("M1 course X"), f.course("M1 course Y")

	first, err := f.orderWithVoucher(os, user, v.Code, courseX)
	if err != nil {
		t.Fatalf("đơn #1: %v", err)
	}
	if err := f.intent(user, first.ID); err != nil {
		t.Fatalf("mã thanh toán đơn #1: %v", err)
	}

	if _, err := f.orderWithVoucher(os, user, v.Code, courseY); !errors.Is(err, ErrVoucherFirstOrderOnly) {
		t.Fatalf("đơn #2 cùng voucher cho khoá khác: err=%v, muốn ErrVoucherFirstOrderOnly", err)
	}
	var withVoucher int64
	f.db.Model(&model.Order{}).Where("user_id = ? AND voucher_id = ?", user, v.ID).Count(&withVoucher)
	if withVoucher != 1 {
		t.Fatalf("có %d đơn dùng voucher, muốn đúng 1", withVoucher)
	}
}

func TestFirstOrderVoucher_RetryCheckoutKeepsOpenOrderError(t *testing.T) {
	f := newOrderFixture(t)
	_, os := newFirstOrderServices(f)
	user := f.user()
	v := firstOrderVoucher(f, true)
	course := f.course("M1 retry")

	first, err := f.orderWithVoucher(os, user, v.Code, course)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.intent(user, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.orderWithVoucher(os, user, v.Code, course); !errors.Is(err, ErrOrderInProgress) {
		t.Fatalf("đặt lại đúng giỏ đang mở: err=%v, muốn ErrOrderInProgress (không phải lỗi đơn đầu tiên)", err)
	}
}
