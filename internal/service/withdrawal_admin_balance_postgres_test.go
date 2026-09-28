package service

// Review Phase 4 (B-1): admin duyệt / đánh dấu đã chuyển KHÔNG được đi qua khi số dư GV âm, kể
// cả khi đơn bị hoàn tiền song song. Postgres thật, schema tạm riêng (newWithdrawalFixture).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func (f *withdrawalFixture) orderService() *AdminOrderService {
	return NewAdminOrderService(repository.NewOrderRepository(f.db), repository.NewOrderItemRepository(f.db),
		repository.NewEnrollmentRepository(f.db), repository.NewCourseRepository(f.db))
}

func (f *withdrawalFixture) refund(orderID uuid.UUID) error {
	_, err := f.orderService().RefundOrder(context.Background(), uuid.New(), orderID, "QA-hoàn tiền", "manual_bank_transfer")
	return err
}

func (f *withdrawalFixture) payoutStatus(id uuid.UUID) string {
	var p model.InstructorPayout
	if err := f.db.Where("id = ?", id).First(&p).Error; err != nil {
		f.t.Fatalf("đọc payout: %v", err)
	}
	return p.Status
}

func mustPayoutNegative(t *testing.T, err error, balance, amount int64) {
	t.Helper()
	if !errors.Is(err, ErrWithdrawalPayoutNegativeBalance) {
		t.Fatalf("err = %v, muốn ErrWithdrawalPayoutNegativeBalance", err)
	}
	d := ruleData(t, err)
	mustEqualDec(t, "data.available_balance", d["available_balance"].(decimal.Decimal), balance)
	mustEqualDec(t, "data.amount", d["amount"].(decimal.Decimal), amount)
}

// Luồng tuần tự qua RefundOrder THẬT (Phase 2).
func TestWithdrawalAdmin_NegativeBalanceBlocksPayout(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	admin := uuid.New()

	t.Run("hoàn tiền rồi duyệt -> 409, từ chối vẫn được", func(t *testing.T) {
		teacher := f.teacher(true)
		a := f.order("completed", 0, orderLine{f.course(teacher), 300000})
		f.order("completed", 0, orderLine{f.course(teacher), 200000})
		w, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(400000))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := f.refund(a); err != nil {
			t.Fatalf("refund: %v", err)
		}
		// Thu nhập còn 200.000, đang giữ 400.000 -> số dư −200.000.
		_, err = f.svc.Approve(ctx, admin, w.ID)
		mustPayoutNegative(t, err, -200000, 400000)
		if s := f.payoutStatus(w.ID); s != model.PayoutStatusPending {
			t.Fatalf("status = %s sau khi bị chặn, muốn pending", s)
		}
		if _, err := f.svc.Reject(ctx, admin, w.ID, "QA-số dư âm"); err != nil {
			t.Fatalf("reject khi số dư âm phải được: %v", err)
		}
	})

	t.Run("hoàn 1 phần mà vẫn đủ -> duyệt được", func(t *testing.T) {
		teacher := f.teacher(true)
		a := f.order("completed", 0, orderLine{f.course(teacher), 300000})
		f.order("completed", 0, orderLine{f.course(teacher), 300000})
		w, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(200000))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := f.refund(a); err != nil {
			t.Fatalf("refund: %v", err)
		}
		if _, err := f.svc.Approve(ctx, admin, w.ID); err != nil {
			t.Fatalf("còn dư 100.000 mà duyệt bị chặn: %v", err)
		}
	})

	t.Run("đã duyệt, hoàn tiền, rồi đánh dấu đã chuyển -> 409", func(t *testing.T) {
		teacher := f.teacher(true)
		a := f.order("completed", 0, orderLine{f.course(teacher), 500000})
		w, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(500000))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := f.svc.Approve(ctx, admin, w.ID); err != nil {
			t.Fatalf("approve: %v", err)
		}
		if err := f.refund(a); err != nil {
			t.Fatalf("refund: %v", err)
		}
		_, err = f.svc.MarkCompleted(ctx, admin, w.ID, "QA-FT-NEG")
		mustPayoutNegative(t, err, -500000, 500000)
		if s := f.payoutStatus(w.ID); s != model.PayoutStatusApproved {
			t.Fatalf("status = %s sau khi bị chặn, muốn approved", s)
		}
	})
}

// holdTeacherProfileLock mở 1 transaction giữ khoá FOR UPDATE trên hồ sơ GV (đóng vai 1 thao tác
// khác đang chạy: hoàn tiền hoặc duyệt), trả transaction và pid backend của nó.
func (f *withdrawalFixture) holdTeacherProfileLock(teacherID uuid.UUID) (*gorm.DB, int) {
	f.t.Helper()
	tx := f.db.Begin()
	var pid int
	if err := tx.Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
		f.t.Fatalf("pg_backend_pid: %v", err)
	}
	if err := tx.Exec("SELECT 1 FROM teacher_profiles WHERE user_id = ? FOR UPDATE", teacherID).Error; err != nil {
		f.t.Fatalf("khoá hồ sơ: %v", err)
	}
	f.t.Cleanup(func() { tx.Rollback() })
	return tx, pid
}

// waitBlockedBy trả true khi có backend đang CHỜ khoá do pid giữ; false nếu thao tác (done) đã
// xong trước — tức là nó KHÔNG hề chờ khoá. Dựa trên pg_blocking_pids nên không phụ thuộc thời gian.
func (f *withdrawalFixture) waitBlockedBy(pid int, done <-chan error) bool {
	f.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			f.t.Logf("thao tác xong mà không chờ khoá (err=%v)", err)
			return false
		default:
		}
		var n int64
		f.db.Raw("SELECT count(*) FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid))", pid).Scan(&n)
		if n > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.t.Fatal("hết giờ chờ: thao tác không xong cũng không chờ khoá")
	return false
}

// Duyệt phải CHỜ khoá hồ sơ GV rồi mới tính số dư: 1 lần hoàn tiền đang giữ khoá commit xong thì
// duyệt thấy số dư mới (âm) -> 409. ĐỎ khi bỏ LockTeacherProfile trong transition.
func TestWithdrawalApprove_WaitsForConcurrentRefund(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	teacher := f.teacher(true)
	a := f.order("completed", 0, orderLine{f.course(teacher), 300000})
	w, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(300000))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	holder, pid := f.holdTeacherProfileLock(teacher)
	done := make(chan error, 1)
	go func() { _, err := f.svc.Approve(ctx, uuid.New(), w.ID); done <- err }()
	if !f.waitBlockedBy(pid, done) {
		t.Fatal("Approve không chờ khoá teacher_profiles: có thể duyệt trên số dư cũ trong lúc hoàn tiền")
	}
	if err := holder.Exec("UPDATE orders SET status = 'refunded' WHERE id = ?", a).Error; err != nil {
		t.Fatalf("hoàn tiền trong transaction giữ khoá: %v", err)
	}
	if err := holder.Commit().Error; err != nil {
		t.Fatalf("commit: %v", err)
	}
	mustPayoutNegative(t, <-done, -300000, 300000)
}

// Hoàn tiền phải CHỜ khoá hồ sơ GV đang bị 1 lần duyệt giữ. ĐỎ khi bỏ
// repository.LockTeacherProfilesOfOrder trong RefundOrder.
func TestRefundOrder_WaitsForWithdrawalPayout(t *testing.T) {
	f := newWithdrawalFixture(t)
	teacher := f.teacher(true)
	a := f.order("completed", 0, orderLine{f.course(teacher), 300000})

	holder, pid := f.holdTeacherProfileLock(teacher)
	done := make(chan error, 1)
	go func() { done <- f.refund(a) }()
	if !f.waitBlockedBy(pid, done) {
		t.Fatal("RefundOrder không chờ khoá teacher_profiles: có thể chen giữa lúc admin duyệt rút tiền")
	}
	holder.Rollback()
	if err := <-done; err != nil {
		t.Fatalf("refund sau khi nhả khoá: %v", err)
	}
}

// Admin B từ chối đúng lúc admin A đang duyệt CÙNG yêu cầu (A giữ khoá dòng, đổi sang approved rồi
// commit): B phải chờ LockByID, đọc trạng thái MỚI và nhận 409 — không được ghi đè approved thành
// rejected. ĐỎ khi bỏ FOR UPDATE trong LockByID: B đọc snapshot cũ "pending", UPDATE chờ khoá rồi
// vẫn ghi đè sau khi A commit (review Phase 4, mutation M7).
func TestWithdrawalReject_WaitsForConcurrentAdminAction(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	teacher := f.teacher(true)
	f.order("completed", 0, orderLine{f.course(teacher), 1000000})
	w, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(300000))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	holder := f.db.Begin()
	t.Cleanup(func() { holder.Rollback() })
	var pid int
	holder.Raw("SELECT pg_backend_pid()").Scan(&pid)
	if err := holder.Exec("SELECT 1 FROM instructor_payouts WHERE id = ? FOR UPDATE", w.ID).Error; err != nil {
		t.Fatalf("khoá yêu cầu: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := f.svc.Reject(ctx, uuid.New(), w.ID, "QA-race"); done <- err }()
	if !f.waitBlockedBy(pid, done) {
		t.Fatal("Reject không chờ khoá dòng yêu cầu")
	}
	if err := holder.Exec("UPDATE instructor_payouts SET status = ? WHERE id = ?", model.PayoutStatusApproved, w.ID).Error; err != nil {
		t.Fatalf("admin A duyệt: %v", err)
	}
	if err := holder.Commit().Error; err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; !errors.Is(err, ErrWithdrawalInvalidTransition) {
		t.Fatalf("Reject sau khi A đã duyệt: err = %v, muốn ErrWithdrawalInvalidTransition", err)
	}
	if s := f.payoutStatus(w.ID); s != model.PayoutStatusApproved {
		t.Fatalf("status = %s, muốn giữ approved (không bị ghi đè)", s)
	}
}

// 2 admin thao tác CÙNG LÚC trên cùng 1 yêu cầu pending (4 duyệt + 4 từ chối): đúng 1 thành công,
// 7 nhận 409 invalid transition, đúng 1 dòng dấu vết admin.
func TestWithdrawalAdmin_ConcurrentActionsOnSameRequest(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	teacher := f.teacher(true)
	f.order("completed", 0, orderLine{f.course(teacher), 1000000})
	w, err := f.svc.Create(ctx, teacher, decimal.NewFromInt(300000))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				_, errs[i] = f.svc.Approve(ctx, uuid.New(), w.ID)
			} else {
				_, errs[i] = f.svc.Reject(ctx, uuid.New(), w.ID, "QA-race")
			}
		}(i)
	}
	close(start)
	wg.Wait()

	ok, conflict := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrWithdrawalInvalidTransition):
			conflict++
		default:
			t.Fatalf("lỗi ngoài dự kiến: %v", err)
		}
	}
	if ok != 1 || conflict != n-1 {
		t.Fatalf("thành công = %d, 409 = %d; muốn 1 và %d", ok, conflict, n-1)
	}
	var p model.InstructorPayout
	f.db.Where("id = ?", w.ID).First(&p)
	if p.Notes == nil || countLines(*p.Notes) != 1 {
		t.Fatalf("notes = %v, muốn đúng 1 dòng dấu vết admin", p.Notes)
	}
}

func countLines(s string) int {
	n := 1
	for _, r := range s {
		if r == '\n' {
			n++
		}
	}
	return n
}
