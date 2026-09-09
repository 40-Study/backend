package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeEnrollmentRepoForFulfillment là fake tối thiểu cho EnrollmentRepositoryInterface, chỉ
// override GetByUserAndCourseUnscoped + RestoreAndReactivate + Create — đủ cho
// completeOrderFulfillment sau khi sửa H2-04 (review vòng 3): hàm giờ dùng bản Unscoped +
// RestoreAndReactivate cho trường hợp mua lại khóa đã unenroll, khớp EnrollmentService.Enroll.
type fakeEnrollmentRepoForFulfillment struct {
	repository.EnrollmentRepositoryInterface
	activeExisting  map[uuid.UUID]bool      // courseID -> đang enroll active (chưa soft-delete)
	deletedExisting map[uuid.UUID]uuid.UUID // courseID -> enrollmentID đã bị soft-delete
	created         []uuid.UUID             // courseID đã Create (enroll lần đầu)
	restored        []uuid.UUID             // enrollmentID đã RestoreAndReactivate
}

func (f *fakeEnrollmentRepoForFulfillment) GetByUserAndCourseUnscoped(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	if f.activeExisting[courseID] {
		return &model.Enrollment{UserID: userID, CourseID: courseID}, nil
	}
	if id, ok := f.deletedExisting[courseID]; ok {
		e := &model.Enrollment{UserID: userID, CourseID: courseID}
		e.ID = id
		e.DeletedAt = gorm.DeletedAt{Valid: true}
		return e, nil
	}
	return nil, nil
}

func (f *fakeEnrollmentRepoForFulfillment) RestoreAndReactivate(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	f.restored = append(f.restored, id)
	return nil
}

func (f *fakeEnrollmentRepoForFulfillment) Create(ctx context.Context, enrollment *model.Enrollment) error {
	f.created = append(f.created, enrollment.CourseID)
	return nil
}

// fakeCourseRepoForFulfillment là fake tối thiểu cho CourseRepositoryInterface, chỉ override
// IncrementTotalStudents (H2-04, review vòng 3) — completeOrderFulfillment TRƯỚC ĐÂY không hề
// gọi hàm này, khiến courses.total_students sai lệch với số enrollment thật.
type fakeCourseRepoForFulfillment struct {
	repository.CourseRepositoryInterface
	incrementedCourseIDs []uuid.UUID
	incrementedDeltas    []int
}

func (f *fakeCourseRepoForFulfillment) IncrementTotalStudents(ctx context.Context, courseID uuid.UUID, delta int) error {
	f.incrementedCourseIDs = append(f.incrementedCourseIDs, courseID)
	f.incrementedDeltas = append(f.incrementedDeltas, delta)
	return nil
}

// fakeVoucherServiceForFulfillment là fake tối thiểu cho VoucherServiceInterface, chỉ override
// RecordUsageLogTx — đủ cho completeOrderFulfillment sau khi sửa H2-05 (review vòng 3):
// IncrementUsedCount KHÔNG còn được gọi ở đây nữa (used_count giờ reserve lúc tạo đơn). H2-06
// vòng 3b: completeOrderFulfillment giờ gọi RecordUsageLogTx (không phải RecordUsageLog) để
// tham gia transaction của caller khi có — fake override đúng method thật được gọi.
type fakeVoucherServiceForFulfillment struct {
	VoucherServiceInterface
	loggedDiscount decimal.Decimal
	logCalled      bool
}

func (f *fakeVoucherServiceForFulfillment) RecordUsageLogTx(ctx context.Context, tx *gorm.DB, voucherID, userID, orderID uuid.UUID, discountAmount decimal.Decimal) error {
	f.logCalled = true
	f.loggedDiscount = discountAmount
	return nil
}

// TestCompleteOrderFulfillment (item 14, review vòng 1; H2-04/H2-05 review vòng 3) — pin lại hợp
// đồng của hàm dùng chung CheckAndProcessPayment VÀ nhánh đơn 0đ tự hoàn tất:
//   - Course CHƯA từng enroll -> Create + IncrementTotalStudents(+1).
//   - Course đã enroll ACTIVE -> bỏ qua hoàn toàn, không Create/Restore/Increment.
//   - Course đã unenroll (soft-delete) -> RestoreAndReactivate + IncrementTotalStudents(+1)
//     (KHÔNG Create — tránh vi phạm unique index idx_user_course, H2-04).
//   - Chỉ ghi RecordUsageLog khi order.VoucherID != nil (KHÔNG còn gọi IncrementUsedCount ở
//     đây nữa — H2-05, used_count reserve lúc tạo đơn).
func TestCompleteOrderFulfillment(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	courseActiveEnrolled := uuid.New()
	courseUnenrolledBefore := uuid.New()
	unenrolledEntryID := uuid.New()
	courseNotEnrolled := uuid.New()
	voucherID := uuid.New()

	t.Run("course chua enroll -> Create + tang total_students", func(t *testing.T) {
		enrollmentRepo := &fakeEnrollmentRepoForFulfillment{
			activeExisting:  map[uuid.UUID]bool{},
			deletedExisting: map[uuid.UUID]uuid.UUID{},
		}
		courseRepo := &fakeCourseRepoForFulfillment{}
		items := []model.OrderItem{{CourseID: courseNotEnrolled}}
		order := &model.Order{ID: orderID, UserID: userID}

		if err := completeOrderFulfillment(context.Background(), nil, enrollmentRepo, courseRepo, nil, items, order); err != nil {
			t.Fatalf("completeOrderFulfillment() unexpected error: %v", err)
		}

		if len(enrollmentRepo.created) != 1 || enrollmentRepo.created[0] != courseNotEnrolled {
			t.Errorf("expected Create called for courseNotEnrolled, got %v", enrollmentRepo.created)
		}
		if len(courseRepo.incrementedCourseIDs) != 1 || courseRepo.incrementedCourseIDs[0] != courseNotEnrolled || courseRepo.incrementedDeltas[0] != 1 {
			t.Errorf("expected IncrementTotalStudents(+1) called for courseNotEnrolled, got ids=%v deltas=%v", courseRepo.incrementedCourseIDs, courseRepo.incrementedDeltas)
		}
	})

	t.Run("course da enroll active -> bo qua hoan toan", func(t *testing.T) {
		enrollmentRepo := &fakeEnrollmentRepoForFulfillment{
			activeExisting:  map[uuid.UUID]bool{courseActiveEnrolled: true},
			deletedExisting: map[uuid.UUID]uuid.UUID{},
		}
		courseRepo := &fakeCourseRepoForFulfillment{}
		items := []model.OrderItem{{CourseID: courseActiveEnrolled}}
		order := &model.Order{ID: orderID, UserID: userID}

		if err := completeOrderFulfillment(context.Background(), nil, enrollmentRepo, courseRepo, nil, items, order); err != nil {
			t.Fatalf("completeOrderFulfillment() unexpected error: %v", err)
		}

		if len(enrollmentRepo.created) != 0 || len(enrollmentRepo.restored) != 0 {
			t.Errorf("expected no Create/RestoreAndReactivate for already-active enrollment, got created=%v restored=%v", enrollmentRepo.created, enrollmentRepo.restored)
		}
		if len(courseRepo.incrementedCourseIDs) != 0 {
			t.Errorf("expected NO total_students increment for already-active enrollment, got %v", courseRepo.incrementedCourseIDs)
		}
	})

	t.Run("H2-04: course da unenroll truoc do -> RestoreAndReactivate (khong Create), tang total_students", func(t *testing.T) {
		enrollmentRepo := &fakeEnrollmentRepoForFulfillment{
			activeExisting:  map[uuid.UUID]bool{},
			deletedExisting: map[uuid.UUID]uuid.UUID{courseUnenrolledBefore: unenrolledEntryID},
		}
		courseRepo := &fakeCourseRepoForFulfillment{}
		items := []model.OrderItem{{CourseID: courseUnenrolledBefore}}
		order := &model.Order{ID: orderID, UserID: userID}

		if err := completeOrderFulfillment(context.Background(), nil, enrollmentRepo, courseRepo, nil, items, order); err != nil {
			t.Fatalf("completeOrderFulfillment() unexpected error: %v", err)
		}

		if len(enrollmentRepo.created) != 0 {
			t.Errorf("expected NO Create (would violate idx_user_course), got %v", enrollmentRepo.created)
		}
		if len(enrollmentRepo.restored) != 1 || enrollmentRepo.restored[0] != unenrolledEntryID {
			t.Errorf("expected RestoreAndReactivate called with id=%v, got %v", unenrolledEntryID, enrollmentRepo.restored)
		}
		if len(courseRepo.incrementedCourseIDs) != 1 || courseRepo.incrementedCourseIDs[0] != courseUnenrolledBefore {
			t.Errorf("expected IncrementTotalStudents(+1) called for restored course, got %v", courseRepo.incrementedCourseIDs)
		}
	})

	t.Run("khong co VoucherID -> khong goi voucherService", func(t *testing.T) {
		enrollmentRepo := &fakeEnrollmentRepoForFulfillment{activeExisting: map[uuid.UUID]bool{}, deletedExisting: map[uuid.UUID]uuid.UUID{}}
		courseRepo := &fakeCourseRepoForFulfillment{}
		voucherSvc := &fakeVoucherServiceForFulfillment{}
		order := &model.Order{ID: orderID, UserID: userID, VoucherID: nil}

		if err := completeOrderFulfillment(context.Background(), nil, enrollmentRepo, courseRepo, voucherSvc, nil, order); err != nil {
			t.Fatalf("completeOrderFulfillment() unexpected error: %v", err)
		}
		if voucherSvc.logCalled {
			t.Errorf("expected voucherService NOT called when order.VoucherID is nil")
		}
	})

	t.Run("H2-05: co VoucherID -> CHI ghi log (khong tang used_count o day nua)", func(t *testing.T) {
		enrollmentRepo := &fakeEnrollmentRepoForFulfillment{activeExisting: map[uuid.UUID]bool{}, deletedExisting: map[uuid.UUID]uuid.UUID{}}
		courseRepo := &fakeCourseRepoForFulfillment{}
		voucherSvc := &fakeVoucherServiceForFulfillment{}
		discount := decimal.RequireFromString("50000")
		order := &model.Order{ID: orderID, UserID: userID, VoucherID: &voucherID, DiscountAmount: discount}

		if err := completeOrderFulfillment(context.Background(), nil, enrollmentRepo, courseRepo, voucherSvc, nil, order); err != nil {
			t.Fatalf("completeOrderFulfillment() unexpected error: %v", err)
		}
		if !voucherSvc.logCalled || !voucherSvc.loggedDiscount.Equal(discount) {
			t.Errorf("expected RecordUsageLog called with discount=%s, got called=%v amount=%s", discount, voucherSvc.logCalled, voucherSvc.loggedDiscount)
		}
	})
}

// fakeOrderHistoryRepoForAlert (vòng 4b, chỉ đạo team-lead — "test: fake fulfillment lỗi -> có
// history fulfillment_failed") — fake tối thiểu cho OrderStatusHistoryRepositoryInterface, chỉ
// override Create để bắt lại history được ghi (hoặc mô phỏng chính bước ghi fallback này cũng
// lỗi, xem test case thứ 2).
type fakeOrderHistoryRepoForAlert struct {
	repository.OrderStatusHistoryRepositoryInterface
	created   []*model.OrderStatusHistory
	createErr error
}

func (f *fakeOrderHistoryRepoForAlert) Create(history *model.OrderStatusHistory) error {
	f.created = append(f.created, history)
	return f.createErr
}

// TestRecordFulfillmentFailureAlert (vòng 4b, chỉ đạo team-lead) — pin hành vi của
// recordFulfillmentFailureAlert (tách ra từ nhánh rollback của CheckAndProcessPayment, xem
// comment tại đó lý do không test được end-to-end qua WithTransaction thật: DummyDialector
// không hỗ trợ db.Transaction thật và panic khi Create() chạy ngoài DryRun — không có
// sqlmock/sqlite trong go.sum):
//   - fake completeOrderFulfillment lỗi (giả lập bằng 1 error tuỳ ý truyền thẳng vào) -> phải
//     ghi ĐÚNG 1 dòng order_status_history với ToStatus="fulfillment_failed", FromStatus=trạng
//     thái cũ của đơn, Reason chứa cả bankTxID lẫn nội dung lỗi gốc (để đối soát thủ công).
//   - nếu chính bước ghi fallback history cũng lỗi -> không panic, không giấu gì thêm (hàm vẫn
//     return bình thường, lỗi gốc do CALLER trả về nguyên vẹn — recordFulfillmentFailureAlert
//     không có giá trị trả về nên chỉ cần verify không panic + Create vẫn được gọi).
func TestRecordFulfillmentFailureAlert(t *testing.T) {
	orderID := uuid.New()
	oldStatus := "processing"
	bankTxID := "FT26099999999"
	amount := "199000"
	fulfillErr := fmt.Errorf("enrollment insert failed: unique constraint violation")

	t.Run("fulfillment loi -> ghi 1 dong history fulfillment_failed", func(t *testing.T) {
		historyRepo := &fakeOrderHistoryRepoForAlert{}
		s := &PaymentService{orderHistoryRepo: historyRepo}

		s.recordFulfillmentFailureAlert(orderID, oldStatus, bankTxID, amount, fulfillErr)

		if len(historyRepo.created) != 1 {
			t.Fatalf("expected exactly 1 history row created, got %d", len(historyRepo.created))
		}
		h := historyRepo.created[0]
		if h.OrderID != orderID {
			t.Errorf("expected OrderID=%s, got %s", orderID, h.OrderID)
		}
		if h.ToStatus != "fulfillment_failed" {
			t.Errorf("expected ToStatus=fulfillment_failed, got %q", h.ToStatus)
		}
		if h.FromStatus != oldStatus {
			t.Errorf("expected FromStatus=%s, got %q", oldStatus, h.FromStatus)
		}
		if !strings.Contains(h.Reason, bankTxID) || !strings.Contains(h.Reason, fulfillErr.Error()) {
			t.Errorf("expected Reason to reference bankTxID and original error, got %q", h.Reason)
		}
	})

	t.Run("ghi fallback history cung loi -> khong panic, van goi Create", func(t *testing.T) {
		historyRepo := &fakeOrderHistoryRepoForAlert{createErr: fmt.Errorf("db down")}
		s := &PaymentService{orderHistoryRepo: historyRepo}

		s.recordFulfillmentFailureAlert(orderID, oldStatus, bankTxID, amount, fulfillErr)

		if len(historyRepo.created) != 1 {
			t.Fatalf("expected Create to still be attempted once, got %d calls", len(historyRepo.created))
		}
	})
}
