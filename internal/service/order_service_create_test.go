package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// newDryRunTxDB (C-01/I-C/I-D/I-E, review vòng 5→6): dựng DB DummyDialector ở chế độ DryRun.
// db.Transaction() THẬT của DummyDialector luôn trả "invalid transaction" và KHÔNG gọi closure
// (đã tự kiểm chứng bằng thực nghiệm trước khi viết bộ test này — go run kiểm tra riêng, không
// phải suy đoán) nên KHÔNG thể test OrderRepository.WithTransaction thật qua DummyDialector.
// fakeOrderRepoForCreateOrder.WithTransaction bên dưới KHÔNG gọi WithTransaction thật — tự dựng
// *repository.OrderRepository trên DB DryRun này rồi gọi closure của CreateOrder TRỰC TIẾP. Dưới
// DryRun, Create/Updates KHÔNG thực thi SQL thật (không panic, luôn trả nil error, đã kiểm bằng
// go run riêng) — đủ để chạy được TOÀN BỘ thân closure THẬT của CreateOrder (không phải giả lập
// lại logic trong test) và pin được cả (a) thứ tự gọi lock/reserve so với insert order (C-01),
// (b) lời gọi LockAndCheckUsagePerUser có thật sự được ĐẤU vào CreateOrder hay không (I-C).
func newDryRunTxDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	return db.Session(&gorm.Session{DryRun: true})
}

// fakeOrderRepoForCreateOrder — implement đúng 2 method CreateOrder thật sự gọi tới trên
// orderRepo: GetExpiredHeldOrdersForUser (sweep) và WithTransaction (khối tạo đơn chính).
type fakeOrderRepoForCreateOrder struct {
	repository.OrderRepositoryInterface
	txDB             *gorm.DB
	expiredOrders    []model.Order
	expiredOrdersErr error
}

func (f *fakeOrderRepoForCreateOrder) GetExpiredHeldOrdersForUser(userID uuid.UUID, ttl time.Duration) ([]model.Order, error) {
	return f.expiredOrders, f.expiredOrdersErr
}

func (f *fakeOrderRepoForCreateOrder) WithTransaction(fn func(repo *repository.OrderRepository) error) error {
	txRepo := repository.NewOrderRepository(f.txDB)
	return fn(txRepo)
}

// fakeCourseRepoForCreateOrder — implement đúng GetByID (CreateOrder chỉ gọi method này qua
// getCoursesByIDs). Đếm số lần gọi để chứng minh CreateOrder có TIẾP TỤC xử lý SAU sweep hay
// không (I-D pin).
type fakeCourseRepoForCreateOrder struct {
	repository.CourseRepositoryInterface
	course *model.Course
	err    error
	calls  int
}

func (f *fakeCourseRepoForCreateOrder) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.course, nil
}

// fakeVoucherServiceForCreateOrder — implement đúng 3 method CreateOrder gọi trên
// voucherService: ValidateAndApplyVoucher (ngoài tx), LockAndCheckUsagePerUser + ReserveVoucherUsage
// (trong tx, I-02/C-01). callOrder (con trỏ dùng chung với registerOrderCreateTracker) ghi lại
// THỨ TỰ THẬT các lời gọi để so sánh với thời điểm order được Create — không suy luận đọc code.
type fakeVoucherServiceForCreateOrder struct {
	VoucherServiceInterface
	voucher     *model.Voucher
	discount    decimal.Decimal
	validateErr error
	lockErr     error
	reserveErr  error
	callOrder   *[]string
}

func (f *fakeVoucherServiceForCreateOrder) ValidateAndApplyVoucher(ctx context.Context, code string, userID uuid.UUID, subtotal decimal.Decimal, paymentMethod string) (*model.Voucher, decimal.Decimal, error) {
	return f.voucher, f.discount, f.validateErr
}

func (f *fakeVoucherServiceForCreateOrder) LockAndCheckUsagePerUser(ctx context.Context, tx *gorm.DB, voucher *model.Voucher, userID uuid.UUID) error {
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, "lock")
	}
	return f.lockErr
}

func (f *fakeVoucherServiceForCreateOrder) ReserveVoucherUsage(ctx context.Context, tx *gorm.DB, voucherID uuid.UUID) error {
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, "reserve")
	}
	return f.reserveErr
}

// registerOrderCreateTracker gắn callback GORM ghi lại THỜI ĐIỂM THẬT CHÍNH order (model.Order,
// phân biệt với order_items/order_status_history qua kiểu Model) được Create — dùng để so sánh
// thứ tự thật với lock/reserve, không phải suy luận đọc code (C-01).
func registerOrderCreateTracker(db *gorm.DB, callOrder *[]string) {
	db.Callback().Create().Before("gorm:create").Register("test:track-order-create", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Model.(*model.Order); ok {
			*callOrder = append(*callOrder, "create-order")
		}
	})
}

func newTestCourse(price decimal.Decimal) *model.Course {
	return &model.Course{
		BaseModel: model.BaseModel{ID: uuid.New()},
		Price:     price,
	}
}

// TestCreateOrder_VoucherLockAndReserveRunBeforeOrderInsert (C-01, review vòng 5→6) — pin ĐÚNG
// thứ tự CreateOrder phải chạy khi có voucher: lock -> reserve -> insert order. TRƯỚC ĐÂY khối
// lock+reserve nằm SAU txRepo.Create(order) — CountUserHeldOrders (chạy TRONG cùng transaction,
// thấy cả ghi CHƯA commit của chính nó) đếm luôn CHÍNH đơn vừa insert, khiến usage_per_user=1
// KHÔNG BAO GIỜ tạo được đơn đầu tiên. Mutation: đảo lại thứ tự (Create trước, lock/reserve sau)
// phải làm test này đỏ.
func TestCreateOrder_VoucherLockAndReserveRunBeforeOrderInsert(t *testing.T) {
	txDB := newDryRunTxDB(t)
	var callOrder []string
	registerOrderCreateTracker(txDB, &callOrder)

	course := newTestCourse(decimal.NewFromInt(100000))
	voucher := &model.Voucher{ID: uuid.New(), Code: "SALE10"}

	svc := &OrderService{
		orderRepo:  &fakeOrderRepoForCreateOrder{txDB: txDB},
		courseRepo: &fakeCourseRepoForCreateOrder{course: course},
		voucherService: &fakeVoucherServiceForCreateOrder{
			voucher: voucher,
			// giảm MỘT PHẦN, KHÔNG về 0đ — tránh nhánh fulfillment đơn 0đ (ngoài phạm vi test này).
			discount:  decimal.NewFromInt(10000),
			callOrder: &callOrder,
		},
	}

	req := dto.CreateOrderRequest{
		Source:     "buy_now",
		CourseIDs:  []string{course.ID.String()},
		CouponCode: "SALE10",
	}

	if _, err := svc.CreateOrder(context.Background(), uuid.New(), req); err != nil {
		t.Fatalf("CreateOrder lỗi không mong đợi: %v", err)
	}

	lockIdx, reserveIdx, createIdx := -1, -1, -1
	for i, c := range callOrder {
		switch c {
		case "lock":
			if lockIdx == -1 {
				lockIdx = i
			}
		case "reserve":
			if reserveIdx == -1 {
				reserveIdx = i
			}
		case "create-order":
			if createIdx == -1 {
				createIdx = i
			}
		}
	}
	if lockIdx == -1 || reserveIdx == -1 || createIdx == -1 {
		t.Fatalf("thiếu 1 trong 3 bước cần thiết, callOrder=%v", callOrder)
	}
	if !(lockIdx < createIdx && reserveIdx < createIdx) {
		t.Errorf("C-01 tái phát: lock/reserve PHẢI chạy TRƯỚC khi insert order (tránh CountUserHeldOrders đếm chính đơn vừa insert) — callOrder=%v", callOrder)
	}
}

// TestCreateOrder_SweepErrorDoesNotBlockOrder (I-D/M-04, review vòng 5→6) — pin hành vi
// "sweep lỗi KHÔNG chặn CreateOrder": GetExpiredHeldOrdersForUser lỗi vẫn phải cho CreateOrder
// TIẾP TỤC xử lý (chứng minh bằng courseRepo.calls > 0, tức đã đi tới bước lấy giá khóa học SAU
// sweep) và trả về thành công. Mutation: đưa lại `return nil, err` khi sweep lỗi phải làm test
// này đỏ (err != nil, courseRepo.calls == 0).
func TestCreateOrder_SweepErrorDoesNotBlockOrder(t *testing.T) {
	txDB := newDryRunTxDB(t)
	course := newTestCourse(decimal.NewFromInt(50000))
	courseRepo := &fakeCourseRepoForCreateOrder{course: course}

	svc := &OrderService{
		orderRepo: &fakeOrderRepoForCreateOrder{
			txDB:             txDB,
			expiredOrdersErr: errors.New("boom: loi sweep gia lap"),
		},
		courseRepo:     courseRepo,
		voucherService: &fakeVoucherServiceForCreateOrder{},
	}

	req := dto.CreateOrderRequest{Source: "buy_now", CourseIDs: []string{course.ID.String()}}

	if _, err := svc.CreateOrder(context.Background(), uuid.New(), req); err != nil {
		t.Fatalf("M-04: lỗi sweep KHÔNG được chặn CreateOrder, nhưng CreateOrder trả lỗi: %v", err)
	}
	if courseRepo.calls == 0 {
		t.Fatalf("kỳ vọng CreateOrder tiếp tục xử lý SAU sweep (gọi courseRepo.GetByID) — courseRepo chưa được gọi, CreateOrder có thể đã return sớm vì lỗi sweep")
	}
}

// TestCreateOrder_VoucherNotApplicableToFreeOrderMessage (I-E/M-06, review vòng 5→6) — pin đúng
// error trả về khi voucher hợp lệ nhưng đơn 0đ (subtotal=0, ValidateAndApplyVoucher trả
// ErrVoucherNotApplicable): CreateOrder PHẢI trả ErrVoucherNotApplicableToFreeOrder (message rõ
// ràng), KHÔNG phải ErrCouponInvalid chung chung. Mutation: đổi/xoá nhánh errors.Is kiểm tra này
// (hoặc đổi biến error trả về) phải làm test đỏ.
func TestCreateOrder_VoucherNotApplicableToFreeOrderMessage(t *testing.T) {
	course := newTestCourse(decimal.Zero)

	svc := &OrderService{
		orderRepo:  &fakeOrderRepoForCreateOrder{},
		courseRepo: &fakeCourseRepoForCreateOrder{course: course},
		voucherService: &fakeVoucherServiceForCreateOrder{
			validateErr: ErrVoucherNotApplicable,
		},
	}

	req := dto.CreateOrderRequest{
		Source:     "buy_now",
		CourseIDs:  []string{course.ID.String()},
		CouponCode: "FREE10",
	}

	_, err := svc.CreateOrder(context.Background(), uuid.New(), req)
	if !errors.Is(err, ErrVoucherNotApplicableToFreeOrder) {
		t.Errorf("M-06: kỳ vọng ErrVoucherNotApplicableToFreeOrder cho đơn 0đ, nhận %v", err)
	}
}
