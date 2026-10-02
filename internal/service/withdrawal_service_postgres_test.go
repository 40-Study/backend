package service

// Phase 4 rút tiền giảng viên — test tích hợp Postgres THẬT.
//
// Vì sao không dùng transaction ROLLBACK như payment_service_platform_fee_integration_test.go: test
// race cần NHIỀU kết nối đồng thời thấy dữ liệu của nhau (khoá dòng chỉ có nghĩa giữa các
// transaction khác nhau). Nên fixture COMMIT dữ liệu thật, nhưng trong 1 schema TẠM riêng
// (pgtest.IsolatedSchema) bị DROP khi test xong. Không có Postgres: Skip ở local, FAIL khi CI=true.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

type withdrawalFixture struct {
	t        *testing.T
	db       *gorm.DB
	svc      *WithdrawalService
	wallet   *WalletService
	users    []uuid.UUID
	courses  []uuid.UUID
	orders   []uuid.UUID
	teachers []uuid.UUID
}

var testMinWithdrawal = decimal.NewFromInt(100000)

// migrateLikeAPIBoot — đúng bước schema lúc API khởi động: database.Migrate (AutoMigrate rồi tự
// gọi RunPostMigrations).
func migrateLikeAPIBoot(db *gorm.DB) error {
	return database.Migrate(db)
}

// isolatedAPISchema — schema đã migrate như lúc API khởi động, rỗng và cô lập cho đúng một test. Dùng lại giữa các
// test của gói thay vì migrate + DROP cho từng test (xem pgtest.ReusableSchema: cô lập được bảo đảm bằng cơ chế
// reset có kiểm lại, không bằng kỷ luật của từng test); PGTEST_REUSE=0 quay về một schema mới cho mỗi test.
func isolatedAPISchema(t *testing.T) *gorm.DB {
	t.Helper()
	return pgtest.ReusableSchema(t, "api-boot", migrateLikeAPIBoot)
}

// newWithdrawalFixture mở 1 schema Postgres TẠM riêng (pgtest.IsolatedSchema), migrate như lúc
// API khởi động, và DROP schema khi test xong: dữ liệu COMMIT (cần cho test race nhiều kết nối)
// không bao giờ chạm schema public của DB dev dùng chung.
func newWithdrawalFixture(t *testing.T) *withdrawalFixture {
	t.Helper()
	db := isolatedAPISchema(t)
	walletRepo := repository.NewWalletRepository(db)
	return &withdrawalFixture{
		t:      t,
		db:     db,
		svc:    NewWithdrawalService(repository.NewWithdrawalRepository(db), walletRepo, testMinWithdrawal),
		wallet: NewWalletService(walletRepo, repository.NewTeacherProfileRepository(db), testMinWithdrawal),
	}
}

func (f *withdrawalFixture) user(kind string) uuid.UUID {
	f.t.Helper()
	s := uuid.NewString()
	u := model.User{Email: "qa-withdrawal-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "QA-withdrawal-" + kind + "-" + s[:8]}
	if err := f.db.Create(&u).Error; err != nil {
		f.t.Fatalf("tạo user: %v", err)
	}
	f.users = append(f.users, u.ID)
	return u.ID
}

// teacher tạo giảng viên (user + teacher_profile), có/không có thông tin ngân hàng.
func (f *withdrawalFixture) teacher(withBank bool) uuid.UUID {
	f.t.Helper()
	id := f.user("teacher")
	p := model.TeacherProfile{UserID: id}
	if withBank {
		bank, num, name := "QA-Vietcombank", "0123456789", "QA NGUYEN VAN A"
		p.BankName, p.BankAccountNumber, p.BankAccountName = &bank, &num, &name
	}
	if err := f.db.Create(&p).Error; err != nil {
		f.t.Fatalf("tạo teacher_profile: %v", err)
	}
	f.teachers = append(f.teachers, id)
	return id
}

func (f *withdrawalFixture) course(teacherID uuid.UUID) uuid.UUID {
	f.t.Helper()
	s := uuid.NewString()
	c := model.Course{InstructorID: teacherID, Title: "QA-withdrawal course " + s[:8], Slug: "qa-withdrawal-" + s, Price: decimal.NewFromInt(1)}
	if err := f.db.Create(&c).Error; err != nil {
		f.t.Fatalf("tạo course: %v", err)
	}
	f.courses = append(f.courses, c.ID)
	return c.ID
}

type orderLine struct {
	course     uuid.UUID
	finalPrice int64
}

// order tạo đơn với phí nền tảng ĐÃ CHỐT (feeAmount) như PaymentService ghi lúc thanh toán.
func (f *withdrawalFixture) order(status string, feeAmount int64, lines ...orderLine) uuid.UUID {
	f.t.Helper()
	student := f.user("student")
	var total int64
	for _, l := range lines {
		total += l.finalPrice
	}
	o := model.Order{
		UserID: student, OrderNumber: "QA-WD-" + uuid.NewString(), Currency: "VND", Status: status,
		Subtotal: decimal.NewFromInt(total), TotalAmount: decimal.NewFromInt(total),
		PlatformFeeAmount: decimal.NewFromInt(feeAmount),
	}
	if err := f.db.Create(&o).Error; err != nil {
		f.t.Fatalf("tạo order: %v", err)
	}
	f.orders = append(f.orders, o.ID)
	for _, l := range lines {
		it := model.OrderItem{OrderID: o.ID, CourseID: l.course, Price: decimal.NewFromInt(l.finalPrice), FinalPrice: decimal.NewFromInt(l.finalPrice)}
		if err := f.db.Create(&it).Error; err != nil {
			f.t.Fatalf("tạo order_item: %v", err)
		}
	}
	return o.ID
}

// payout chèn thẳng 1 yêu cầu ở trạng thái bất kỳ (dựng tình huống, không đi qua service).
func (f *withdrawalFixture) payout(teacherID uuid.UUID, status string, amount int64) {
	f.t.Helper()
	p := model.InstructorPayout{InstructorID: teacherID, Amount: decimal.NewFromInt(amount), Currency: "VND", Status: status}
	if err := f.db.Create(&p).Error; err != nil {
		f.t.Fatalf("tạo payout %s: %v", status, err)
	}
}

func (f *withdrawalFixture) countPayouts(teacherID uuid.UUID, status string) int64 {
	var n int64
	f.db.Model(&model.InstructorPayout{}).Where("instructor_id = ? AND status = ?", teacherID, status).Count(&n)
	return n
}

func mustEqualDec(t *testing.T, label string, got decimal.Decimal, want int64) {
	t.Helper()
	if !got.Equal(decimal.NewFromInt(want)) {
		t.Fatalf("%s = %s, muốn %d", label, got, want)
	}
}

func ruleData(t *testing.T, err error) map[string]interface{} {
	t.Helper()
	var re *WithdrawalRuleError
	if !errors.As(err, &re) {
		t.Fatalf("lỗi %v không phải WithdrawalRuleError", err)
	}
	return re.Data
}

// ─── Số dư ─────────────────────────────────────────────────────────────────

// Số dư = phần GV (sau phí nền tảng CHỐT theo đơn) của đơn completed − yêu cầu pending/approved/
// completed. Đơn refunded/pending không tính, yêu cầu rejected không trừ. Đơn nhiều giảng viên
// phân bổ phí theo tỉ lệ final_price. Kiểm cả 2 đường tính (ví giáo viên và danh sách admin gom
// nhóm) phải ra CÙNG số.
func TestWithdrawalBalance_Formula(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	teacher := f.teacher(true)
	other := f.teacher(true)
	c1, c3 := f.course(teacher), f.course(teacher)
	c2 := f.course(other)

	f.order("completed", 100000, orderLine{c1, 1000000})                      // GV: 1.000.000 − 100.000 = 900.000
	f.order("completed", 50000, orderLine{c3, 300000}, orderLine{c2, 700000}) // GV: 300.000 − 15.000 = 285.000
	f.order("refunded", 0, orderLine{c1, 500000})                             // đã hoàn: không tính
	f.order("pending", 0, orderLine{c1, 500000})                              // chưa thanh toán: không tính

	f.payout(teacher, model.PayoutStatusCompleted, 200000)
	f.payout(teacher, model.PayoutStatusRejected, 300000) // bị từ chối: KHÔNG trừ
	f.payout(teacher, model.PayoutStatusApproved, 100000)

	w, err := f.wallet.GetTeacherWallet(ctx, teacher)
	if err != nil {
		t.Fatalf("GetTeacherWallet: %v", err)
	}
	mustEqualDec(t, "total_earnings", w.TotalEarnings, 1185000)
	mustEqualDec(t, "total_paid_out", w.TotalPaidOut, 200000)
	mustEqualDec(t, "pending_withdrawal", w.PendingWithdrawal, 100000)
	mustEqualDec(t, "available_balance", w.AvailBalance, 885000)
	mustEqualDec(t, "min_withdrawal_amount", w.MinWithdrawalAmount, 100000)
	if !w.HasOpenWithdrawal {
		t.Fatal("has_open_withdrawal = false dù có yêu cầu approved")
	}

	list, err := f.svc.AdminList(ctx, &teacher, "", 1, 20)
	if err != nil {
		t.Fatalf("AdminList: %v", err)
	}
	if len(list.Items) != 3 {
		t.Fatalf("AdminList trả %d mục, muốn 3", len(list.Items))
	}
	mustEqualDec(t, "teacher_available_balance (admin)", list.Items[0].TeacherAvailableBalance, 885000)

	// Giảng viên kia chỉ nhận phần của mình trong đơn chung: 700.000 − 35.000.
	wo, err := f.wallet.GetTeacherWallet(ctx, other)
	if err != nil {
		t.Fatalf("GetTeacherWallet(other): %v", err)
	}
	mustEqualDec(t, "total_earnings (other)", wo.TotalEarnings, 665000)
}

// ─── Tạo yêu cầu: các quy tắc ──────────────────────────────────────────────

func TestWithdrawalCreate_Rules(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()

	t.Run("dưới mức tối thiểu", func(t *testing.T) {
		teacher := f.teacher(true)
		f.order("completed", 0, orderLine{f.course(teacher), 1000000})
		_, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(99999))
		if !errors.Is(err, ErrWithdrawalBelowMinimum) {
			t.Fatalf("err = %v, muốn ErrWithdrawalBelowMinimum", err)
		}
		mustEqualDec(t, "min_amount", ruleData(t, err)["min_amount"].(decimal.Decimal), 100000)
		if n := f.countPayouts(teacher, model.PayoutStatusPending); n != 0 {
			t.Fatalf("đã ghi %d yêu cầu dù bị từ chối", n)
		}
		// Đúng bằng mức tối thiểu thì được.
		if _, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(100000)); err != nil {
			t.Fatalf("rút đúng 100.000 bị từ chối: %v", err)
		}
	})

	t.Run("đã có 1 yêu cầu đang chờ", func(t *testing.T) {
		teacher := f.teacher(true)
		f.order("completed", 0, orderLine{f.course(teacher), 1000000})
		first, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(200000))
		if err != nil {
			t.Fatalf("yêu cầu 1: %v", err)
		}
		_, err = f.svc.Create(ctx, teacher, decimal.NewFromInt(200000))
		if !errors.Is(err, ErrWithdrawalAlreadyOpen) {
			t.Fatalf("yêu cầu 2 err = %v, muốn ErrWithdrawalAlreadyOpen (còn dư 800.000 nhưng chỉ 1 yêu cầu đang chờ)", err)
		}
		if got := ruleData(t, err)["id"]; got != first.ID {
			t.Fatalf("data.id = %v, muốn id yêu cầu đang chờ %v", got, first.ID)
		}
		// approved vẫn tính là đang xử lý.
		if _, err := f.svc.Approve(ctx, uuid.New(), first.ID); err != nil {
			t.Fatalf("approve: %v", err)
		}
		if _, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(200000)); !errors.Is(err, ErrWithdrawalAlreadyOpen) {
			t.Fatalf("khi yêu cầu 1 đang approved, err = %v, muốn ErrWithdrawalAlreadyOpen", err)
		}
	})

	t.Run("số dư âm chặn rút", func(t *testing.T) {
		teacher := f.teacher(true)
		orderID := f.order("completed", 0, orderLine{f.course(teacher), 300000})
		f.payout(teacher, model.PayoutStatusCompleted, 300000) // đã rút hết
		// Đơn bị hoàn tiền SAU khi đã rút (Phase 2) -> thu nhập giảm 300.000 -> số dư −300.000.
		if err := f.db.Model(&model.Order{}).Where("id = ?", orderID).Update("status", "refunded").Error; err != nil {
			t.Fatalf("refund: %v", err)
		}
		// Có thêm doanh thu mới nhưng chưa bù đủ: −300.000 + 250.000 = −50.000.
		f.order("completed", 0, orderLine{f.course(teacher), 250000})

		w, err := f.wallet.GetTeacherWallet(ctx, teacher)
		if err != nil {
			t.Fatalf("GetTeacherWallet: %v", err)
		}
		mustEqualDec(t, "available_balance", w.AvailBalance, -50000)

		_, err = f.svc.Create(ctx, teacher, decimal.NewFromInt(100000))
		if !errors.Is(err, ErrWithdrawalNegativeBalance) {
			t.Fatalf("err = %v, muốn ErrWithdrawalNegativeBalance", err)
		}
		mustEqualDec(t, "data.available_balance", ruleData(t, err)["available_balance"].(decimal.Decimal), -50000)

		neg, err := f.svc.NegativeBalances(ctx)
		if err != nil {
			t.Fatalf("NegativeBalances: %v", err)
		}
		found := false
		for _, it := range neg.Items {
			if it.TeacherID == teacher {
				found = true
				mustEqualDec(t, "negative-balances.available_balance", it.AvailableBalance, -50000)
			}
		}
		if !found {
			t.Fatal("giảng viên số dư âm không có trong cảnh báo admin")
		}

		// Doanh thu mới bù lại: 250.000 nữa -> +200.000 -> rút được.
		f.order("completed", 0, orderLine{f.course(teacher), 250000})
		if _, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(200000)); err != nil {
			t.Fatalf("sau khi bù đủ vẫn bị chặn: %v", err)
		}
	})

	t.Run("vượt số dư", func(t *testing.T) {
		teacher := f.teacher(true)
		f.order("completed", 0, orderLine{f.course(teacher), 150000})
		_, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(200000))
		if !errors.Is(err, ErrWithdrawalInsufficientBalance) {
			t.Fatalf("err = %v, muốn ErrWithdrawalInsufficientBalance", err)
		}
		mustEqualDec(t, "data.available_balance", ruleData(t, err)["available_balance"].(decimal.Decimal), 150000)
	})

	t.Run("thiếu thông tin ngân hàng", func(t *testing.T) {
		teacher := f.teacher(false)
		f.order("completed", 0, orderLine{f.course(teacher), 1000000})
		if _, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(200000)); !errors.Is(err, ErrWithdrawalBankInfoRequired) {
			t.Fatalf("err = %v, muốn ErrWithdrawalBankInfoRequired", err)
		}
	})

	t.Run("không có hồ sơ giáo viên", func(t *testing.T) {
		student := f.user("student")
		if _, err := f.svc.Create(ctx, student, decimal.NewFromInt(200000)); !errors.Is(err, ErrWithdrawalTeacherProfileRequired) {
			t.Fatalf("err = %v, muốn ErrWithdrawalTeacherProfileRequired", err)
		}
	})
}

// ─── Race ──────────────────────────────────────────────────────────────────

// 8 request rút cùng lúc của CÙNG 1 giảng viên: chỉ đúng 1 thành công, 7 cái còn lại nhận
// ErrWithdrawalAlreadyOpen, và DB chỉ có đúng 1 yêu cầu pending. Test phải ĐỎ khi bỏ khoá
// LockTeacherProfile (các transaction cùng đọc "chưa có yêu cầu" rồi cùng INSERT).
func TestWithdrawalCreate_ConcurrentRequests_OnlyOneSucceeds(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	teacher := f.teacher(true)
	f.order("completed", 0, orderLine{f.course(teacher), 1000000})

	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = f.svc.Create(ctx, teacher, decimal.NewFromInt(200000))
		}(i)
	}
	close(start)
	wg.Wait()

	ok, open := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrWithdrawalAlreadyOpen):
			open++
		default:
			t.Fatalf("lỗi ngoài dự kiến: %v", err)
		}
	}
	if ok != 1 || open != n-1 {
		t.Fatalf("thành công = %d, already_open = %d; muốn 1 và %d", ok, open, n-1)
	}
	if c := f.countPayouts(teacher, model.PayoutStatusPending); c != 1 {
		t.Fatalf("DB có %d yêu cầu pending, muốn 1", c)
	}
}

// ─── Admin ─────────────────────────────────────────────────────────────────

func TestWithdrawalAdmin_ApproveRejectComplete(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	admin := uuid.New()
	teacher := f.teacher(true)
	f.order("completed", 0, orderLine{f.course(teacher), 1000000})

	// pending -> approved -> completed
	w1, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(500000))
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	if _, err := f.svc.MarkCompleted(ctx, admin, w1.ID, "QA-FT001"); !errors.Is(err, ErrWithdrawalInvalidTransition) {
		t.Fatalf("mark-completed từ pending: err = %v, muốn ErrWithdrawalInvalidTransition", err)
	}
	if res, err := f.svc.Approve(ctx, admin, w1.ID); err != nil || res.Status != model.PayoutStatusApproved {
		t.Fatalf("approve: res=%v err=%v", res, err)
	}
	if _, err := f.svc.MarkCompleted(ctx, admin, w1.ID, "   "); !errors.Is(err, ErrWithdrawalTransactionIDRequired) {
		t.Fatalf("mark-completed thiếu mã GD: err = %v", err)
	}
	if res, err := f.svc.MarkCompleted(ctx, admin, w1.ID, "QA-FT001"); err != nil || res.Status != model.PayoutStatusCompleted {
		t.Fatalf("mark-completed: res=%v err=%v", res, err)
	}
	var saved model.InstructorPayout
	f.db.Where("id = ?", w1.ID).First(&saved)
	if saved.TransactionID == nil || *saved.TransactionID != "QA-FT001" || saved.ProcessedAt == nil {
		t.Fatalf("mark-completed không lưu transaction_id/processed_at: %+v", saved)
	}
	if _, err := f.svc.Approve(ctx, admin, w1.ID); !errors.Is(err, ErrWithdrawalInvalidTransition) {
		t.Fatalf("approve từ completed: err = %v", err)
	}

	w, _ := f.wallet.GetTeacherWallet(ctx, teacher)
	mustEqualDec(t, "total_paid_out sau khi chuyển", w.TotalPaidOut, 500000)
	mustEqualDec(t, "available_balance sau khi chuyển", w.AvailBalance, 500000)

	// pending -> rejected: giải phóng số dư ngay
	w2, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(500000))
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	w, _ = f.wallet.GetTeacherWallet(ctx, teacher)
	mustEqualDec(t, "available_balance khi đang chờ", w.AvailBalance, 0)
	if _, err := f.svc.Reject(ctx, admin, w2.ID, "  "); !errors.Is(err, ErrWithdrawalReasonRequired) {
		t.Fatalf("reject không lý do: err = %v", err)
	}
	if res, err := f.svc.Reject(ctx, admin, w2.ID, "QA-sai tên chủ tài khoản"); err != nil || res.Status != model.PayoutStatusRejected {
		t.Fatalf("reject: res=%v err=%v", res, err)
	}
	var rejected model.InstructorPayout
	f.db.Where("id = ?", w2.ID).First(&rejected)
	if rejected.RejectionReason == nil || *rejected.RejectionReason != "QA-sai tên chủ tài khoản" || rejected.ProcessedAt == nil {
		t.Fatalf("reject không lưu lý do/processed_at: %+v", rejected)
	}
	if _, err := f.svc.Approve(ctx, admin, w2.ID); !errors.Is(err, ErrWithdrawalInvalidTransition) {
		t.Fatalf("approve từ rejected: err = %v", err)
	}
	w, _ = f.wallet.GetTeacherWallet(ctx, teacher)
	mustEqualDec(t, "available_balance sau khi từ chối", w.AvailBalance, 500000)

	// Số dư đã được giải phóng: rút lại đúng toàn bộ phần còn lại phải được.
	if _, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(500000)); err != nil {
		t.Fatalf("rút lại sau khi bị từ chối: %v", err)
	}

	if _, err := f.svc.Approve(ctx, admin, uuid.New()); !errors.Is(err, ErrWithdrawalNotFound) {
		t.Fatalf("approve id không tồn tại: err = %v", err)
	}
}

// ─── Quyền sở hữu ──────────────────────────────────────────────────────────

// Giáo viên chỉ thấy yêu cầu của chính mình: ListMine lọc theo teacherID lấy từ token.
func TestWithdrawalListMine_OnlyOwnRequests(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	a, b := f.teacher(true), f.teacher(true)
	f.order("completed", 0, orderLine{f.course(a), 1000000})
	f.order("completed", 0, orderLine{f.course(b), 1000000})
	wa, err := f.svc.Create(ctx, a, decimal.NewFromInt(100000))
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	wb, err := f.svc.Create(ctx, b, decimal.NewFromInt(300000))
	if err != nil {
		t.Fatalf("create b: %v", err)
	}

	for _, tc := range []struct {
		who  uuid.UUID
		want uuid.UUID
	}{{a, wa.ID}, {b, wb.ID}} {
		res, err := f.svc.ListMine(ctx, tc.who, "", 1, 20)
		if err != nil {
			t.Fatalf("ListMine: %v", err)
		}
		if res.TotalCount != 1 || len(res.Items) != 1 || res.Items[0].ID != tc.want {
			t.Fatalf("ListMine(%s) = %+v, muốn đúng 1 yêu cầu %s", tc.who, res.Items, tc.want)
		}
	}
}
