package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeOrderRepoForCreatePaymentIntent (I6-02, review vòng 6→7) — implement đúng GetByID (trả về
// đơn "processing" + payment code CÒN HẠN) và WithTransaction (chỉ ghi nhận có bị gọi hay không,
// không cần chạy closure thật) — đủ để pin fast-path B-03: "processing" + code còn hạn phải trả
// THẲNG code cũ, KHÔNG bao giờ chạm tới WithTransaction (tức KHÔNG gọi UpdatePaymentCode/sinh
// code mới).
type fakeOrderRepoForCreatePaymentIntent struct {
	repository.OrderRepositoryInterface
	order                 *model.Order
	withTransactionCalled bool
}

func (f *fakeOrderRepoForCreatePaymentIntent) GetByID(id uuid.UUID) (*model.Order, error) {
	return f.order, nil
}

func (f *fakeOrderRepoForCreatePaymentIntent) WithTransaction(fn func(repo *repository.OrderRepository) error) error {
	f.withTransactionCalled = true
	return nil
}

// TestCreatePaymentIntent_ProcessingWithValidCode_ReturnsExistingCodeWithoutRegenerating
// (I6-02, review vòng 6→7) — review vòng 6 tự chạy MUT-4c (xoá fast-path "tái dùng code còn
// hạn") và thấy suite vẫn XANH vì KHÔNG có test nào pin nửa hành vi NGƯỜI DÙNG THẤY này (chỉ có
// test SQL-shape cho UpdatePaymentCode ở tầng repository, không có test ở tầng CreatePaymentIntent
// xác nhận fast-path THỰC SỰ được đấu vào). Mutation: xoá khối `if order.Status == "processing"
// && ...` phải làm test này đỏ (rơi xuống nhánh `order.Status != "pending"` -> ErrInvalidStateTransition).
func TestCreatePaymentIntent_ProcessingWithValidCode_ReturnsExistingCodeWithoutRegenerating(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	oldCode := "OLD-CODE-KEEP-ME"
	expiresAt := time.Now().Add(2 * time.Hour) // còn hạn

	order := &model.Order{
		ID:                   orderID,
		UserID:               userID,
		Status:               "processing",
		PaymentTransactionID: &oldCode,
		PaymentCodeExpiredAt: &expiresAt,
		TotalAmount:          decimal.NewFromInt(100000),
		Currency:             "VND",
	}

	fakeRepo := &fakeOrderRepoForCreatePaymentIntent{order: order}
	s := &PaymentService{orderRepo: fakeRepo}

	resp, err := s.CreatePaymentIntent(context.Background(), userID, orderID, false, "bank_transfer")
	if err != nil {
		t.Fatalf("CreatePaymentIntent lỗi không mong đợi: %v", err)
	}
	if resp.PaymentCode != oldCode {
		t.Errorf("B-03 fast-path tái phát: kỳ vọng trả lại code CŨ %q, nhận %q (sinh code MỚI thay vì tái dùng)", oldCode, resp.PaymentCode)
	}
	if fakeRepo.withTransactionCalled {
		t.Errorf("B-03 fast-path tái phát: KHÔNG được gọi WithTransaction/UpdatePaymentCode khi code cũ còn hạn, nhưng đã gọi")
	}
}

// TestCreatePaymentIntent_ProcessingWithExpiredCode_FailsInsteadOfFastPath (I6-02, đối chứng) —
// code HẾT HẠN không được đi qua fast-path (điều kiện `time.Now().Before(*ExpiredAt)` phải chặn
// đúng) — rơi xuống guard `order.Status != "pending"` -> ErrInvalidStateTransition (chủ đích,
// user phải tạo đơn mới; xem review vòng 6 mục B-03 "Đơn processing + code HẾT HẠN").
func TestCreatePaymentIntent_ProcessingWithExpiredCode_FailsInsteadOfFastPath(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	oldCode := "EXPIRED-CODE"
	expiresAt := time.Now().Add(-1 * time.Hour) // đã hết hạn

	order := &model.Order{
		ID:                   orderID,
		UserID:               userID,
		Status:               "processing",
		PaymentTransactionID: &oldCode,
		PaymentCodeExpiredAt: &expiresAt,
		TotalAmount:          decimal.NewFromInt(100000),
		Currency:             "VND",
	}

	fakeRepo := &fakeOrderRepoForCreatePaymentIntent{order: order}
	s := &PaymentService{orderRepo: fakeRepo}

	_, err := s.CreatePaymentIntent(context.Background(), userID, orderID, false, "bank_transfer")
	if err != ErrInvalidStateTransition {
		t.Errorf("kỳ vọng ErrInvalidStateTransition cho code đã hết hạn (không được lọt qua fast-path), nhận %v", err)
	}
}
