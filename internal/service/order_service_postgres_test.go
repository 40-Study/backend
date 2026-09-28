package service

// QA vòng 2, lane B (đơn hàng) — test tích hợp Postgres THẬT cho OrderService.
//
// Vì sao Postgres thật: CreateOrder chạy trong transaction + khoá advisory (pg_advisory_xact_lock),
// DummyDialector không chạy được transaction; test race cần nhiều kết nối commit thật. Fixture
// dùng pgtest.IsolatedSchema (schema tạm, DROP khi xong) giống withdrawal_service_postgres_test.go.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

type orderFixture struct {
	t   *testing.T
	db  *gorm.DB
	svc *OrderService
}

func newOrderFixture(t *testing.T) *orderFixture {
	t.Helper()
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := NewOrderService(
		repository.NewOrderRepository(db),
		repository.NewOrderItemRepository(db),
		repository.NewCourseRepository(db),
		repository.NewCartItemRepository(db),
		nil, // idempotency: test gửi key khác nhau để chắc chắn KHÔNG đi qua nhánh replay
		nil, // voucher: các test này không dùng mã giảm giá
	)
	return &orderFixture{t: t, db: db, svc: svc}
}

func (f *orderFixture) user() uuid.UUID {
	f.t.Helper()
	s := uuid.NewString()
	u := model.User{Email: "qa-order-" + s + "@40study.test", PasswordHash: "x", UserName: "QA-order-" + s[:8]}
	if err := f.db.Create(&u).Error; err != nil {
		f.t.Fatalf("tạo user: %v", err)
	}
	return u.ID
}

func (f *orderFixture) course(title string) uuid.UUID {
	f.t.Helper()
	teacher := f.user()
	s := uuid.NewString()
	c := model.Course{InstructorID: teacher, Title: title, Slug: "qa-order-" + s, Price: decimal.NewFromInt(499000)}
	if err := f.db.Create(&c).Error; err != nil {
		f.t.Fatalf("tạo course: %v", err)
	}
	return c.ID
}

func buyNow(courseIDs ...uuid.UUID) dto.CreateOrderRequest {
	ids := make([]string, 0, len(courseIDs))
	for _, id := range courseIDs {
		ids = append(ids, id.String())
	}
	return dto.CreateOrderRequest{Source: "buy_now", CourseIDs: ids, IdempotencyKey: uuid.NewString()}
}

func (f *orderFixture) createOrder(userID uuid.UUID, courseIDs ...uuid.UUID) *dto.OrderResponse {
	f.t.Helper()
	resp, err := f.svc.CreateOrder(context.Background(), userID, buyNow(courseIDs...))
	if err != nil {
		f.t.Fatalf("CreateOrder: %v", err)
	}
	return resp
}

func (f *orderFixture) exec(sql string, args ...interface{}) {
	f.t.Helper()
	if err := f.db.Exec(sql, args...).Error; err != nil {
		f.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (f *orderFixture) dbCreatedAt(orderID uuid.UUID) time.Time {
	f.t.Helper()
	var o model.Order
	if err := f.db.Where("id = ?", orderID).First(&o).Error; err != nil {
		f.t.Fatalf("đọc order: %v", err)
	}
	return o.CreatedAt
}

func (f *orderFixture) countOpenOrders(userID uuid.UUID) int64 {
	var n int64
	f.db.Model(&model.Order{}).Where("user_id = ? AND status IN ('pending','processing')", userID).Count(&n)
	return n
}

// B1: ngày tạo đơn phải là created_at trong DB, đọc nhiều lần ra cùng giá trị; hạn giữ đơn pending
// = created_at + 24h, KHÔNG tự lùi theo thời điểm đọc.
func TestOrderResponse_CreatedAtAndExpiresAtComeFromDB(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	order := f.createOrder(student, f.course("QA-order B1"))

	// Đẩy created_at về 2 giờ trước: nếu response còn dùng time.Now() thì sẽ lệch ~2 giờ.
	createdAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	f.exec("UPDATE orders SET created_at = ? WHERE id = ?", createdAt, order.ID)
	stored := f.dbCreatedAt(order.ID)

	first, err := f.svc.GetOrderByID(context.Background(), order.ID, student, false)
	if err != nil {
		t.Fatalf("GetOrderByID lần 1: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	second, err := f.svc.GetOrderByID(context.Background(), order.ID, student, false)
	if err != nil {
		t.Fatalf("GetOrderByID lần 2: %v", err)
	}

	for i, r := range []*dto.OrderResponse{first, second} {
		if !r.CreatedAt.Equal(stored) {
			t.Fatalf("lần đọc %d: created_at = %s, muốn giá trị DB %s", i+1, r.CreatedAt, stored)
		}
		if r.ExpiresAt == nil || !r.ExpiresAt.Equal(stored.Add(pendingOrderDefaultTTL)) {
			t.Fatalf("lần đọc %d: expires_at = %v, muốn created_at + 24h = %s", i+1, r.ExpiresAt, stored.Add(pendingOrderDefaultTTL))
		}
	}

	// Danh sách "Đơn hàng của tôi" cũng phải cùng mốc.
	list, err := f.svc.GetUserOrders(context.Background(), student, 1, 10, "")
	if err != nil {
		t.Fatalf("GetUserOrders: %v", err)
	}
	if len(list.Orders) != 1 || !list.Orders[0].CreatedAt.Equal(stored) {
		t.Fatalf("GetUserOrders created_at sai: %+v", list.Orders)
	}
}

// B1: đơn đã mở phiên thanh toán (processing) hết hạn đúng theo payment_code_expired_at đã lưu.
func TestOrderResponse_ProcessingExpiresAtIsStoredPaymentCodeExpiry(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	order := f.createOrder(student, f.course("QA-order B1 processing"))

	codeExpiry := time.Now().Add(3 * time.Hour).Truncate(time.Microsecond)
	f.exec("UPDATE orders SET status = 'processing', payment_transaction_id = 'QA-CODE', payment_code_expired_at = ? WHERE id = ?", codeExpiry, order.ID)

	r, err := f.svc.GetOrderByID(context.Background(), order.ID, student, false)
	if err != nil {
		t.Fatalf("GetOrderByID: %v", err)
	}
	if r.ExpiresAt == nil || !r.ExpiresAt.Equal(codeExpiry) {
		t.Fatalf("expires_at = %v, muốn payment_code_expired_at %s", r.ExpiresAt, codeExpiry)
	}
}

// B2: course_name phải có ở chi tiết đơn (cũng là nguồn của trang admin) và /orders/me.
func TestOrderItems_CourseNameIsFilled(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	const title = "QA-order B2 tên khoá"
	order := f.createOrder(student, f.course(title))

	detail, err := f.svc.GetOrderByID(context.Background(), order.ID, uuid.Nil, true)
	if err != nil {
		t.Fatalf("GetOrderByID (admin): %v", err)
	}
	if len(detail.Items) != 1 || detail.Items[0].CourseName != title {
		t.Fatalf("chi tiết đơn: course_name = %+v, muốn %q", detail.Items, title)
	}

	list, err := f.svc.GetUserOrders(context.Background(), student, 1, 10, "")
	if err != nil {
		t.Fatalf("GetUserOrders: %v", err)
	}
	if len(list.Orders) != 1 || len(list.Orders[0].Items) != 1 || list.Orders[0].Items[0].CourseName != title {
		t.Fatalf("/orders/me: course_name sai: %+v", list.Orders)
	}
}

// B4: bấm "Mua ngay" lần 2 cho cùng khoá khi đơn cũ còn pending → trả lại ĐÚNG đơn cũ.
func TestCreateOrder_DuplicatePendingReturnsExistingOrder(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	course := f.course("QA-order B4 pending")

	first := f.createOrder(student, course)
	second := f.createOrder(student, course)
	if second.ID != first.ID {
		t.Fatalf("tạo đơn trùng khoá tạo đơn mới %s, muốn trả lại đơn cũ %s", second.ID, first.ID)
	}
	if n := f.countOpenOrders(student); n != 1 {
		t.Fatalf("số đơn mở = %d, muốn 1", n)
	}
}

// B4: đơn cũ đã mở phiên thanh toán (processing, mã còn hạn) → 409 ErrOrderInProgress.
func TestCreateOrder_DuplicateProcessingIsConflict(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	course := f.course("QA-order B4 processing")
	first := f.createOrder(student, course)
	f.exec("UPDATE orders SET status = 'processing', payment_transaction_id = 'QA-CODE', payment_code_expired_at = ? WHERE id = ?", time.Now().Add(time.Hour), first.ID)

	_, err := f.svc.CreateOrder(context.Background(), student, buyNow(course))
	if !errors.Is(err, ErrOrderInProgress) {
		t.Fatalf("err = %v, muốn ErrOrderInProgress", err)
	}
	if n := f.countOpenOrders(student); n != 1 {
		t.Fatalf("số đơn mở = %d, muốn 1", n)
	}
}

// B4: đơn cũ chứa khoá này nhưng khác tập khoá (vd giỏ 2 khoá) → không trả nhầm đơn giá khác, 409.
func TestCreateOrder_OverlappingDifferentSetIsConflict(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	a, b := f.course("QA-order B4 A"), f.course("QA-order B4 B")
	f.createOrder(student, a, b)

	_, err := f.svc.CreateOrder(context.Background(), student, buyNow(a))
	if !errors.Is(err, ErrOrderInProgress) {
		t.Fatalf("err = %v, muốn ErrOrderInProgress", err)
	}
}

// B4: đơn cũ đã quá hạn giữ thì không chặn mua lại; khoá khác thì không liên quan.
func TestCreateOrder_ExpiredOrOtherCourseDoesNotBlock(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	course, other := f.course("QA-order B4 expired"), f.course("QA-order B4 other")
	old := f.createOrder(student, course)
	f.exec("UPDATE orders SET created_at = ? WHERE id = ?", time.Now().Add(-pendingOrderDefaultTTL-time.Hour), old.ID)

	fresh := f.createOrder(student, course)
	if fresh.ID == old.ID {
		t.Fatalf("đơn đã quá hạn vẫn bị trả lại")
	}
	if f.createOrder(student, other).ID == fresh.ID {
		t.Fatalf("khoá khác lại trả về đơn của khoá trước")
	}
}

// B4 race: nhiều request "Mua ngay" đồng thời (double-click, nhiều tab), mỗi request một
// idempotency key khác nhau → chỉ được có ĐÚNG 1 đơn mở, mọi request thành công trả cùng 1 đơn.
func TestCreateOrder_ConcurrentSameCourseCreatesSingleOrder(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	course := f.course("QA-order B4 race")

	const n = 8
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		ids   = map[uuid.UUID]int{}
		errs  []error
		start = make(chan struct{})
	)

	// Barrier giữa bước "kiểm đơn trùng" và bước "ghi đơn": mỗi request đứng chờ tới khi đủ n
	// request cùng qua bước kiểm, tối đa 300ms. Không có khoá thì cả n request cùng thấy "chưa có
	// đơn" rồi cùng ghi (đỏ chắc chắn, không phụ thuộc may rủi lịch goroutine). Có khoá advisory
	// thì chỉ 1 request ở trong vùng này mỗi lúc, barrier hết giờ và request đi tiếp tuần tự.
	var arrived sync.WaitGroup
	arrived.Add(n)
	var arrivedCount atomic.Int32
	afterOpenOrderCheckHook = func() {
		// Chỉ Done tối đa n lần (WaitGroup âm sẽ panic).
		if arrivedCount.Add(1) <= n {
			arrived.Done()
		}
		done := make(chan struct{})
		go func() { arrived.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(300 * time.Millisecond):
		}
	}
	t.Cleanup(func() { afterOpenOrderCheckHook = nil })
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := f.svc.CreateOrder(context.Background(), student, buyNow(course))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ids[resp.ID]++
		}()
	}
	close(start)
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("có request lỗi: %v", errs)
	}
	if got := f.countOpenOrders(student); got != 1 {
		t.Fatalf("%d request đồng thời tạo %d đơn mở, muốn 1 (ids: %v)", n, got, ids)
	}
	if len(ids) != 1 {
		t.Fatalf("các request trả về %d đơn khác nhau, muốn 1: %v", len(ids), ids)
	}
}
