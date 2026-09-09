package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeOrderRepoForCheckAndProcessPayment (I6-03, review vòng 6→7) — WithTransaction bỏ qua
// closure thật (không cần chạy RecordBankTransactionUsage/UpdatePaymentInfo/history/fulfillment —
// những bước đó đã được test riêng ở nơi khác), CHỈ trả thẳng `withTransactionErr` đã cấu hình —
// đủ để pin đúng phần QUYẾT ĐỊNH SAU KHI transaction thất bại: có alert hay không (call site
// `if !shouldAlertFulfillmentFailure(err)`), không phải logic bên TRONG transaction.
type fakeOrderRepoForCheckAndProcessPayment struct {
	repository.OrderRepositoryInterface
	order                 *model.Order
	withTransactionErr    error
	withTransactionCalled bool
}

func (f *fakeOrderRepoForCheckAndProcessPayment) GetByID(id uuid.UUID) (*model.Order, error) {
	return f.order, nil
}

func (f *fakeOrderRepoForCheckAndProcessPayment) WithTransaction(fn func(repo *repository.OrderRepository) error) error {
	f.withTransactionCalled = true
	return f.withTransactionErr
}

// fakeTransactionServiceForCheckAndProcessPayment — trả về 1 giao dịch ngân hàng "Found=true",
// số tiền khớp CHÍNH XÁC order.TotalAmount (để đi qua guard amount-match, tới được WithTransaction).
type fakeTransactionServiceForCheckAndProcessPayment struct {
	result *grpc.CheckTransactionResult
}

func (f *fakeTransactionServiceForCheckAndProcessPayment) CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*grpc.CheckTransactionResult, error) {
	return f.result, nil
}

func (f *fakeTransactionServiceForCheckAndProcessPayment) IsHealthy(ctx context.Context) (bool, error) {
	return true, nil
}

// fakeOrderHistoryRepoForCheckAndProcessPayment — ghi nhận có bị gọi Create hay không (dùng để
// phát hiện recordFulfillmentFailureAlert CÓ chạy hay KHÔNG — không cần assert nội dung history,
// chỉ cần biết ALERT CÓ xảy ra hay không).
type fakeOrderHistoryRepoForCheckAndProcessPayment struct {
	repository.OrderStatusHistoryRepositoryInterface
	created []*model.OrderStatusHistory
}

func (f *fakeOrderHistoryRepoForCheckAndProcessPayment) Create(history *model.OrderStatusHistory) error {
	f.created = append(f.created, history)
	return nil
}

func newTestOrderForCheckAndProcessPayment(userID, orderID uuid.UUID) (*model.Order, string) {
	code := "PAYCODE123"
	expiresAt := time.Now().Add(1 * time.Hour)
	order := &model.Order{
		ID:                   orderID,
		UserID:               userID,
		Status:               "processing",
		PaymentTransactionID: &code,
		PaymentCodeExpiredAt: &expiresAt,
		TotalAmount:          decimal.NewFromInt(100000),
		Currency:             "VND",
	}
	return order, code
}

// TestCheckAndProcessPayment_BenignErrPaymentAlreadyDone_DoesNotAlert (I6-03, review vòng 6→7)
// — review vòng 6 tự chạy MUT-5b (vô hiệu hoá call site `if !shouldAlertFulfillmentFailure(err)`
// bằng `if false`) và thấy suite vẫn XANH — hàm thuần shouldAlertFulfillmentFailure CÓ test
// (TestShouldAlertFulfillmentFailure) nhưng CALL SITE thật trong CheckAndProcessPayment thì
// KHÔNG (đúng mẫu "present but not wired"). Test này pin: khi transaction thất bại vì
// ErrPaymentAlreadyDone (benign — request khác đã xử lý giao dịch này rồi), KHÔNG được ghi
// history "fulfillment_failed"/gọi alert.
func TestCheckAndProcessPayment_BenignErrPaymentAlreadyDone_DoesNotAlert(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	order, code := newTestOrderForCheckAndProcessPayment(userID, orderID)

	orderRepo := &fakeOrderRepoForCheckAndProcessPayment{order: order, withTransactionErr: ErrPaymentAlreadyDone}
	historyRepo := &fakeOrderHistoryRepoForCheckAndProcessPayment{}
	txSvc := &fakeTransactionServiceForCheckAndProcessPayment{result: &grpc.CheckTransactionResult{
		Found: true, TransactionID: "BANKTX1", Amount: "100000",
	}}

	s := &PaymentService{orderRepo: orderRepo, orderHistoryRepo: historyRepo, transactionService: txSvc}

	_, err := s.CheckAndProcessPayment(context.Background(), orderID, userID, false)
	if !errors.Is(err, ErrPaymentAlreadyDone) {
		t.Fatalf("kỳ vọng lỗi ErrPaymentAlreadyDone lan ra, nhận %v (code dùng: %s)", err, code)
	}
	if len(historyRepo.created) != 0 {
		t.Errorf("I6-03 tái phát: lỗi benign ErrPaymentAlreadyDone KHÔNG được kích hoạt alert (ghi history fulfillment_failed), nhưng đã ghi %d bản ghi", len(historyRepo.created))
	}
}

// TestCheckAndProcessPayment_OtherFulfillmentError_Alerts (I6-03, đối chứng chiều ngược lại) —
// lỗi KHÁC ErrPaymentAlreadyDone (thường gặp nhất: completeOrderFulfillment lỗi) PHẢI kích hoạt
// alert (ghi history "fulfillment_failed" qua recordFulfillmentFailureAlert) — nếu không, tiền
// chuyển khoản thật đã amount-match nhưng rollback hết mà KHÔNG có dấu vết gì để ops đối soát.
func TestCheckAndProcessPayment_OtherFulfillmentError_Alerts(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	order, _ := newTestOrderForCheckAndProcessPayment(userID, orderID)

	fulfillmentErr := errors.New("enrollment insert failed")
	orderRepo := &fakeOrderRepoForCheckAndProcessPayment{order: order, withTransactionErr: fulfillmentErr}
	historyRepo := &fakeOrderHistoryRepoForCheckAndProcessPayment{}
	txSvc := &fakeTransactionServiceForCheckAndProcessPayment{result: &grpc.CheckTransactionResult{
		Found: true, TransactionID: "BANKTX2", Amount: "100000",
	}}

	s := &PaymentService{orderRepo: orderRepo, orderHistoryRepo: historyRepo, transactionService: txSvc}

	_, err := s.CheckAndProcessPayment(context.Background(), orderID, userID, false)
	if !errors.Is(err, fulfillmentErr) {
		t.Fatalf("kỳ vọng lỗi fulfillment gốc lan ra, nhận %v", err)
	}
	if len(historyRepo.created) != 1 {
		t.Fatalf("I6-03 tái phát: lỗi fulfillment KHÔNG-benign PHẢI kích hoạt alert (ghi 1 history fulfillment_failed), nhưng ghi %d bản ghi", len(historyRepo.created))
	}
	if historyRepo.created[0].ToStatus != "fulfillment_failed" {
		t.Errorf("kỳ vọng ToStatus=fulfillment_failed, nhận %q", historyRepo.created[0].ToStatus)
	}
}
