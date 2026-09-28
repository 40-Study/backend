package service

// Review doi khang (260928, finding #1 MAJOR): khong test nao — truoc test nay — di qua DUONG
// THANH TOAN THAT (CheckAndProcessPayment, ham DUY NHAT trong repo goi SetPlatformFeeSnapshot voi
// du lieu that) de khang dinh cot phi nen tang duoc CHOT. Cac test san co
// (payment_service_platform_fee_test.go) chi pin ham THUAN calculatePlatformFeeAmount, khong lap
// rap qua CheckAndProcessPayment; mutation test cua reviewer (doi `if s.platformSettingRepo !=
// nil {` thanh `if false {`) chung minh 100% suite cu van XANH du xoa het khoi chot phi.
//
// Vi sao test nay dung Postgres THAT (khong DryRun/DummyDialector): OrderRepository.WithTransaction
// goi r.db.Transaction(...) — DummyDialector khong ho tro transaction that (tu kiem chung bang
// script probe: tra loi "invalid transaction", KHONG goi closure). Neu dung DummyDialector,
// CheckAndProcessPayment se loi ngay o buoc WithTransaction dau tien, khong bao gio toi duoc doan
// chot phi can test. Theo dung quy uoc da co san trong repo (xem
// internal/repository/note_repository_postgres_test.go, R5) — ket noi Postgres dev that, tao du
// lieu trong 1 transaction ROLLBACK cuoi cung, bo qua (t.Skip) neu khong ket noi duoc.
import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

// fakeTransactionServiceMatched — TransactionServiceInterface gia lap, luon tra ve 1 giao dich
// ngan hang KHOP so tien voi don (Found=true), mo phong duong THAT (webhook/polling ngan hang) ma
// khong can goi gRPC that.
type fakeTransactionServiceMatched struct {
	amount string
	txID   string
}

func (f *fakeTransactionServiceMatched) CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*grpc.CheckTransactionResult, error) {
	return &grpc.CheckTransactionResult{
		Found:         true,
		TransactionID: f.txID,
		Amount:        f.amount,
		Currency:      "VND",
	}, nil
}

func (f *fakeTransactionServiceMatched) IsHealthy(ctx context.Context) (bool, error) {
	return true, nil
}

// TestCheckAndProcessPayment_ChotPhiNenTangDungThoiDiemThanhToan (review 260928, finding #1
// MAJOR): mo phong dung DUONG THANH TOAN THAT — tao 1 don "processing" that trong Postgres, dat %
// phi nen tang = 8% qua chinh PlatformSettingRepository (khong tu ghi thang vao cot), goi
// CheckAndProcessPayment That voi transactionService gia lap tra ve giao dich khop tien. Sau khi
// ham chay xong, doc lai don TU DB (khong doc lai bien in-memory) va khang dinh
// platform_fee_percent/platform_fee_amount da duoc CHOT dung 8% / 63920 (799000 * 8%). Neu khoi
// chot phi trong CheckAndProcessPayment bi xoa (mutation cua reviewer), 2 cot nay se van la 0 —
// test PHAI do.
func TestCheckAndProcessPayment_ChotPhiNenTangDungThoiDiemThanhToan(t *testing.T) {
	db := openTestPostgresForPaymentTest(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	defer sqlDB.Close()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("db.Begin(): %v", tx.Error)
	}
	// Test tich hop: KHONG ghi gi xuong DB that — rollback toan bo o cuoi, ke ca UPDATE phi nen
	// tang tam thoi dat qua PlatformSettingRepository.
	defer tx.Rollback()

	suffix := uuid.NewString()
	instructor := model.User{
		Email:        "fee-it-instructor-" + suffix + "@40study.test",
		PasswordHash: "x",
		UserName:     "fee-it-instructor-" + suffix,
	}
	if err := tx.Create(&instructor).Error; err != nil {
		t.Fatalf("tao instructor: %v", err)
	}
	student := model.User{
		Email:        "fee-it-student-" + suffix + "@40study.test",
		PasswordHash: "x",
		UserName:     "fee-it-student-" + suffix,
	}
	if err := tx.Create(&student).Error; err != nil {
		t.Fatalf("tao student: %v", err)
	}
	course := model.Course{
		InstructorID: instructor.ID,
		Title:        "Fee IT course " + suffix,
		Slug:         "fee-it-course-" + suffix,
		Price:        decimal.NewFromInt(799000),
	}
	if err := tx.Create(&course).Error; err != nil {
		t.Fatalf("tao course: %v", err)
	}

	paymentCode := "PAYFEEIT" + suffix[:8]
	expiredAt := time.Now().Add(24 * time.Hour) // con han — khong roi vao nhanh "expired"
	order := model.Order{
		UserID:               student.ID,
		OrderNumber:          "ORD-FEE-IT-" + suffix,
		Subtotal:             decimal.NewFromInt(799000),
		TotalAmount:          decimal.NewFromInt(799000),
		Currency:             "VND",
		Status:               "processing",
		PaymentTransactionID: &paymentCode,
		PaymentCodeExpiredAt: &expiredAt,
	}
	if err := tx.Create(&order).Error; err != nil {
		t.Fatalf("tao order: %v", err)
	}
	orderItem := model.OrderItem{
		OrderID:    order.ID,
		CourseID:   course.ID,
		Price:      decimal.NewFromInt(799000),
		FinalPrice: decimal.NewFromInt(799000),
	}
	if err := tx.Create(&orderItem).Error; err != nil {
		t.Fatalf("tao order_item: %v", err)
	}

	platformSettingRepo := repository.NewPlatformSettingRepository(tx)
	if err := platformSettingRepo.SetPlatformFeePercent(context.Background(), decimal.NewFromInt(8), instructor.ID); err != nil {
		t.Fatalf("set ty le phi nen tang = 8 phan tram: %v", err)
	}

	orderRepo := repository.NewOrderRepository(tx)
	fakeTxService := &fakeTransactionServiceMatched{
		amount: "799000",
		txID:   "BANKTX-FEE-IT-" + suffix,
	}

	// enrollmentRepo/orderHistoryRepo/voucherService: nil — don khong co voucher, va test nay chi
	// quan tam cot phi nen tang tren "orders" (khong lap lai pham vi cac test khac da co san cho
	// enrollment/history — TestCheckAndProcessPayment_* rieng neu can, xem finding #1 chi yeu cau
	// dung duong that, khong yeu cau lap toan bo side-effect).
	paymentService := NewPaymentService(
		orderRepo,
		nil, // orderHistoryRepo — chi dung khi fulfillment loi, khong roi vao nhanh do o day
		nil, // enrollmentRepo — bo qua completeOrderFulfillment, tap trung dung vao snapshot phi
		nil, // voucherService — don khong co voucher
		fakeTxService,
		platformSettingRepo,
	)

	_, err = paymentService.CheckAndProcessPayment(context.Background(), order.ID, student.ID, false)
	if err != nil {
		t.Fatalf("CheckAndProcessPayment tra loi = %v, muon thanh cong (giao dich da khop tien)", err)
	}

	var reloaded model.Order
	if err := tx.Where("id = ?", order.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("doc lai don tu DB: %v", err)
	}

	if reloaded.Status != "completed" {
		t.Fatalf("don status = %q, muon \"completed\"", reloaded.Status)
	}

	wantPercent := decimal.NewFromInt(8)
	if !reloaded.PlatformFeePercent.Equal(wantPercent) {
		t.Fatalf("platform_fee_percent = %s, muon %s — chot phi KHONG chay qua duong thanh toan that (kiem tra khoi `if s.platformSettingRepo != nil` trong CheckAndProcessPayment)",
			reloaded.PlatformFeePercent.String(), wantPercent.String())
	}

	wantAmount := decimal.NewFromInt(63920) // 799000 * 8% = 63920
	if !reloaded.PlatformFeeAmount.Equal(wantAmount) {
		t.Fatalf("platform_fee_amount = %s, muon %s (799000 * 8%%) — chot phi KHONG chay qua duong thanh toan that",
			reloaded.PlatformFeeAmount.String(), wantAmount.String())
	}
}

// openTestPostgresForPaymentTest — ket noi Postgres qua pgtest.Open: bo qua khi chay local khong
// co DB, nhung FAIL khi CI=true (review Phase 4, B-2). Test tu ROLLBACK nen khong de lai du lieu.
func openTestPostgresForPaymentTest(t *testing.T) *gorm.DB {
	t.Helper()
	return pgtest.Open(t)
}
