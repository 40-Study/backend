package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeEnrollmentRepoForFulfillment là fake tối thiểu cho EnrollmentRepositoryInterface, chỉ
// override GetByUserAndCourse + Create — đủ cho completeOrderFulfillment.
type fakeEnrollmentRepoForFulfillment struct {
	repository.EnrollmentRepositoryInterface
	existing map[uuid.UUID]bool // courseID -> đã enroll
	created  []uuid.UUID        // courseID đã Create
}

func (f *fakeEnrollmentRepoForFulfillment) GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	if f.existing[courseID] {
		return &model.Enrollment{UserID: userID, CourseID: courseID}, nil
	}
	return nil, nil
}

func (f *fakeEnrollmentRepoForFulfillment) Create(ctx context.Context, enrollment *model.Enrollment) error {
	f.created = append(f.created, enrollment.CourseID)
	return nil
}

// fakeVoucherServiceForFulfillment là fake tối thiểu cho VoucherServiceInterface, chỉ override
// IncrementUsedCount + RecordUsageLog — đủ cho completeOrderFulfillment.
type fakeVoucherServiceForFulfillment struct {
	VoucherServiceInterface
	incrementedVoucherID uuid.UUID
	incrementCalled      bool
	loggedDiscount       decimal.Decimal
	logCalled            bool
}

func (f *fakeVoucherServiceForFulfillment) IncrementUsedCount(ctx context.Context, voucherID uuid.UUID) error {
	f.incrementCalled = true
	f.incrementedVoucherID = voucherID
	return nil
}

func (f *fakeVoucherServiceForFulfillment) RecordUsageLog(ctx context.Context, voucherID, userID, orderID uuid.UUID, discountAmount decimal.Decimal) error {
	f.logCalled = true
	f.loggedDiscount = discountAmount
	return nil
}

// TestCompleteOrderFulfillment (item 14, review vòng 1) — pin lại hợp đồng của hàm dùng chung
// CheckAndProcessPayment VÀ nhánh đơn 0đ tự hoàn tất: tạo enrollment cho course CHƯA enroll,
// bỏ qua course ĐÃ enroll, và chỉ ghi nhận usage voucher khi order.VoucherID != nil.
func TestCompleteOrderFulfillment(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	courseAlreadyEnrolled := uuid.New()
	courseNotEnrolled := uuid.New()
	voucherID := uuid.New()

	t.Run("tao enrollment cho course chua enroll, bo qua course da enroll", func(t *testing.T) {
		enrollmentRepo := &fakeEnrollmentRepoForFulfillment{
			existing: map[uuid.UUID]bool{courseAlreadyEnrolled: true},
		}
		items := []model.OrderItem{
			{CourseID: courseAlreadyEnrolled},
			{CourseID: courseNotEnrolled},
		}
		order := &model.Order{ID: orderID, UserID: userID}

		if err := completeOrderFulfillment(context.Background(), enrollmentRepo, nil, items, order); err != nil {
			t.Fatalf("completeOrderFulfillment() unexpected error: %v", err)
		}

		if len(enrollmentRepo.created) != 1 || enrollmentRepo.created[0] != courseNotEnrolled {
			t.Errorf("expected only courseNotEnrolled to be created, got %v", enrollmentRepo.created)
		}
	})

	t.Run("khong co VoucherID -> khong goi voucherService", func(t *testing.T) {
		enrollmentRepo := &fakeEnrollmentRepoForFulfillment{existing: map[uuid.UUID]bool{}}
		voucherSvc := &fakeVoucherServiceForFulfillment{}
		order := &model.Order{ID: orderID, UserID: userID, VoucherID: nil}

		if err := completeOrderFulfillment(context.Background(), enrollmentRepo, voucherSvc, nil, order); err != nil {
			t.Fatalf("completeOrderFulfillment() unexpected error: %v", err)
		}
		if voucherSvc.incrementCalled || voucherSvc.logCalled {
			t.Errorf("expected voucherService NOT called when order.VoucherID is nil")
		}
	})

	t.Run("co VoucherID -> tang used_count va ghi log dung discount", func(t *testing.T) {
		enrollmentRepo := &fakeEnrollmentRepoForFulfillment{existing: map[uuid.UUID]bool{}}
		voucherSvc := &fakeVoucherServiceForFulfillment{}
		discount := decimal.RequireFromString("50000")
		order := &model.Order{ID: orderID, UserID: userID, VoucherID: &voucherID, DiscountAmount: discount}

		if err := completeOrderFulfillment(context.Background(), enrollmentRepo, voucherSvc, nil, order); err != nil {
			t.Fatalf("completeOrderFulfillment() unexpected error: %v", err)
		}
		if !voucherSvc.incrementCalled || voucherSvc.incrementedVoucherID != voucherID {
			t.Errorf("expected IncrementUsedCount called with voucherID=%v, got called=%v id=%v", voucherID, voucherSvc.incrementCalled, voucherSvc.incrementedVoucherID)
		}
		if !voucherSvc.logCalled || !voucherSvc.loggedDiscount.Equal(discount) {
			t.Errorf("expected RecordUsageLog called with discount=%s, got called=%v amount=%s", discount, voucherSvc.logCalled, voucherSvc.loggedDiscount)
		}
	})
}
