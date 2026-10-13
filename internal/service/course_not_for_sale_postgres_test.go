package service

// Review QA2 H2 (quyết định chủ dự án 13/10): khoá `archived` (gỡ khỏi bán, gồm cả khi admin gỡ từ báo cáo) NGỪNG BÁN:
// không thêm vào giỏ, không mua (cart + buy_now), không mở phiên thanh toán cho đơn cũ, không tự ghi danh miễn phí - lỗi
// rõ ràng COURSE_NOT_FOR_SALE (409). Người ĐÃ ghi danh vẫn học được. Postgres thật.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func requireNotForSale(t *testing.T, label string, err error) {
	t.Helper()
	var known *apperr.KnownError
	if !errors.As(err, &known) || known.Status != http.StatusConflict || known.Code != "COURSE_NOT_FOR_SALE" || known.Message == "" {
		t.Fatalf("%s: err = %v, muon KnownError 409 COURSE_NOT_FOR_SALE", label, err)
	}
}

func archivedOrderCourse(f *orderFixture) uuid.UUID {
	id := f.course("QA khoa ngung ban")
	f.exec("UPDATE courses SET status = 'archived' WHERE id = ?", id)
	return id
}

func TestArchivedCourse_BuyNowVaOrderTuGio_BiChan(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	student := f.user()
	course := archivedOrderCourse(f)

	_, err := f.svc.CreateOrder(ctx, student, buyNow(course))
	requireNotForSale(t, "buy_now", err)

	// Khoá vào giỏ TRƯỚC khi bị gỡ (giỏ cũ) cũng không checkout được.
	f.exec("UPDATE courses SET status = 'published' WHERE id = ?", course)
	if err := repository.NewCartItemRepository(f.db).Create(ctx, &model.CartItem{UserID: student, CourseID: course}); err != nil {
		t.Fatalf("them gio: %v", err)
	}
	f.exec("UPDATE courses SET status = 'archived' WHERE id = ?", course)
	_, err = f.svc.CreateOrder(ctx, student, dto.CreateOrderRequest{Source: "cart", IdempotencyKey: uuid.NewString()})
	requireNotForSale(t, "cart checkout", err)

	if n := f.countOpenOrders(student); n != 0 {
		t.Errorf("khong duoc tao don, co %d", n)
	}
}

func TestArchivedCourse_ThemVaoGioVaGhiDanhMienPhi_BiChan(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	student := f.user()
	course := archivedOrderCourse(f)

	cartSvc := NewCartService(repository.NewCartItemRepository(f.db), repository.NewCourseRepository(f.db), repository.NewEnrollmentRepository(f.db))
	_, err := cartSvc.AddToCart(ctx, student, dto.AddToCartDTO{CourseID: course})
	requireNotForSale(t, "add to cart", err)

	f.exec("UPDATE courses SET price = 0 WHERE id = ?", course)
	_, err = f.enrollmentService().Enroll(ctx, student, course)
	requireNotForSale(t, "free enroll", err)
	var n int64
	f.db.Model(&model.Enrollment{}).Where("user_id = ? AND course_id = ?", student, course).Count(&n)
	if n != 0 {
		t.Errorf("khong duoc ghi danh, co %d dong", n)
	}
}

func TestArchivedCourse_DonPendingCu_KhongMoDuocThanhToan(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	student := f.user()
	course := f.course("QA don cu")
	order := f.createOrder(student, course)
	f.exec("UPDATE courses SET status = 'archived' WHERE id = ?", course)

	_, err := f.paymentService().CreatePaymentIntent(ctx, student, order.ID, false, "bank_transfer")
	requireNotForSale(t, "payment intent", err)
	if o := f.loadOrder(order.ID); o.Status != "pending" {
		t.Errorf("don phai giu pending (khong cap ma), la %q", o.Status)
	}
}

func TestArchivedCourse_NguoiDaGhiDanhVanVaoHoc_NguoiLaThiKhong(t *testing.T) {
	f := newOrderFixture(t)
	owner, enrolled, stranger := f.user(), f.user(), f.user()
	course := model.Course{InstructorID: owner, Status: model.CourseStatusArchived, Price: decimal.Zero}

	if canViewCourse(&course, stranger, false, false) {
		t.Error("nguoi la khong duoc xem khoa archived")
	}
	for name, ok := range map[string]bool{
		"nguoi da ghi danh": canViewCourse(&course, enrolled, false, true),
		"chu khoa":          canViewCourse(&course, owner, false, false),
		"admin":             canViewCourse(&course, stranger, true, false),
	} {
		if !ok {
			t.Errorf("%s phai xem duoc khoa archived", name)
		}
	}
}
