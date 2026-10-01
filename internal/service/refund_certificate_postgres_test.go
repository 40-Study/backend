package service

// Lane P: hoàn tiền một đơn mua khoá phải thu hồi chứng chỉ của khoá đó (bảng certificates).
// Postgres thật, schema tạm riêng (newOrderFixture). Chứng nhận cuộc thi nằm ở contest_awards,
// bảng khác và không đi qua đường này nên không bị đụng.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// refundCertFixture: đơn ĐÃ HOÀN TẤT của học viên cho một khoá, ghi danh đã học xong và chứng chỉ
// đã cấp (qua đúng đường cấp bù của CertificateService).
type refundCertFixture struct {
	*orderFixture
	certSvc *CertificateService
	student uuid.UUID
	course  uuid.UUID
	order   uuid.UUID
	number  string
}

func newRefundCertFixture(t *testing.T) *refundCertFixture {
	t.Helper()
	f := newOrderFixture(t)
	rc := &refundCertFixture{orderFixture: f, student: f.user(), course: f.course("QA-P chứng chỉ hoàn tiền")}
	rc.certSvc = NewCertificateService(repository.NewCertificateRepository(f.db), repository.NewEnrollmentRepository(f.db), nil, nil)

	rc.order = rc.completeOrderFor(rc.course)
	completedAt := time.Now().Add(-time.Hour)
	e := model.Enrollment{UserID: rc.student, CourseID: rc.course, EnrolledAt: completedAt.Add(-time.Hour), CompletedAt: &completedAt, ProgressPercent: decimal.NewFromInt(100)}
	if err := f.db.Create(&e).Error; err != nil {
		t.Fatalf("tạo enrollment: %v", err)
	}
	list, err := rc.certSvc.GetMyCertificates(context.Background(), rc.student, 1, 10)
	if err != nil || list.Total != 1 {
		t.Fatalf("cấp chứng chỉ: %+v err=%v, muốn đúng 1", list, err)
	}
	rc.number = list.Data[0].CertificateNumber
	return rc
}

func (rc *refundCertFixture) completeOrderFor(course uuid.UUID) uuid.UUID {
	rc.t.Helper()
	order := rc.createOrder(rc.student, course)
	rc.exec("UPDATE orders SET status = 'completed', paid_at = now() WHERE id = ?", order.ID)
	return order.ID
}

func (rc *refundCertFixture) refund(orderID uuid.UUID) {
	rc.t.Helper()
	if _, err := rc.adminOrderService().RefundOrder(context.Background(), uuid.New(), orderID, "QA-P hoàn tiền", "manual_bank_transfer", "FT-QA-P"); err != nil {
		rc.t.Fatalf("RefundOrder: %v", err)
	}
}

// Hoàn tiền: chứng chỉ không còn trong danh sách, không xem được theo id, và trang tra cứu công
// khai báo "đã thu hồi" mà không lộ tên người học / tên khoá.
func TestRefundOrder_RevokesCourseCertificate(t *testing.T) {
	rc := newRefundCertFixture(t)
	ctx := context.Background()

	stored, err := repository.NewCertificateRepository(rc.db).GetCertificateByNumber(ctx, rc.number)
	if err != nil || stored == nil || stored.RevokedAt != nil {
		t.Fatalf("trước khi hoàn: cert=%+v err=%v, muốn tồn tại và chưa thu hồi", stored, err)
	}
	if v, err := rc.certSvc.VerifyCertificate(ctx, rc.number); err != nil || !v.Valid {
		t.Fatalf("trước khi hoàn: verify=%+v err=%v, muốn hợp lệ", v, err)
	}

	rc.refund(rc.order)

	list, err := rc.certSvc.GetMyCertificates(ctx, rc.student, 1, 10)
	if err != nil || list.Total != 0 || len(list.Data) != 0 {
		t.Fatalf("sau hoàn tiền: list=%+v err=%v, muốn 0 chứng chỉ", list, err)
	}
	if _, err := rc.certSvc.GetCertificateByID(ctx, stored.ID, rc.student, false); err == nil {
		t.Fatalf("GetCertificateByID chứng chỉ đã thu hồi: muốn lỗi not found")
	}
	v, err := rc.certSvc.VerifyCertificate(ctx, rc.number)
	if err != nil || v.Valid || !v.Revoked || v.UserName != "" || v.CourseName != "" {
		t.Fatalf("verify sau hoàn tiền: %+v err=%v, muốn valid=false revoked=true, không lộ tên", v, err)
	}
	// Giữ dòng để tra cứu vẫn phân biệt "đã thu hồi" với "không tồn tại".
	after, _ := repository.NewCertificateRepository(rc.db).GetCertificateByNumber(ctx, rc.number)
	if after == nil || after.RevokedAt == nil {
		t.Fatalf("dòng chứng chỉ sau hoàn tiền: %+v, muốn còn dòng và có revoked_at", after)
	}
}

// Còn một đơn hoàn tất KHÁC của cùng (user, khoá) thì khoá học vẫn "sống": không thu hồi chứng chỉ
// (cùng điều kiện với việc gỡ ghi danh).
func TestRefundOrder_KeepsCertificateWhileAnotherCompletedOrderRemains(t *testing.T) {
	rc := newRefundCertFixture(t)
	ctx := context.Background()
	rc.completeOrderFor(rc.course) // đơn hoàn tất thứ hai cho cùng khoá

	rc.refund(rc.order)

	list, err := rc.certSvc.GetMyCertificates(ctx, rc.student, 1, 10)
	if err != nil || list.Total != 1 {
		t.Fatalf("còn đơn hoàn tất khác: list=%+v err=%v, muốn giữ 1 chứng chỉ", list, err)
	}
	if v, err := rc.certSvc.VerifyCertificate(ctx, rc.number); err != nil || !v.Valid {
		t.Fatalf("verify: %+v err=%v, muốn vẫn hợp lệ", v, err)
	}
}

// Chứng chỉ của khoá KHÁC (cùng học viên) không bị đụng.
func TestRefundOrder_LeavesOtherCoursesCertificateAlone(t *testing.T) {
	rc := newRefundCertFixture(t)
	ctx := context.Background()

	otherCourse := rc.course2()
	completedAt := time.Now().Add(-time.Hour)
	e := model.Enrollment{UserID: rc.student, CourseID: otherCourse, EnrolledAt: completedAt.Add(-time.Hour), CompletedAt: &completedAt}
	if err := rc.db.Create(&e).Error; err != nil {
		t.Fatalf("tạo enrollment khoá khác: %v", err)
	}
	if list, err := rc.certSvc.GetMyCertificates(ctx, rc.student, 1, 10); err != nil || list.Total != 2 {
		t.Fatalf("cấp chứng chỉ khoá thứ hai: %+v err=%v, muốn 2", list, err)
	}

	rc.refund(rc.order)

	list, err := rc.certSvc.GetMyCertificates(ctx, rc.student, 1, 10)
	if err != nil || list.Total != 1 || list.Data[0].CourseID != otherCourse {
		t.Fatalf("sau hoàn tiền khoá 1: %+v err=%v, muốn còn đúng chứng chỉ khoá %s", list, err, otherCourse)
	}
}

func (rc *refundCertFixture) course2() uuid.UUID {
	return rc.orderFixture.course("QA-P khoá thứ hai")
}

// Mua lại rồi học xong lần nữa thì chứng chỉ cũ được cấp lại đúng số; chưa học xong thì vẫn bị
// thu hồi (mua lại không tự trả chứng chỉ).
func TestRefundOrder_CertificateIsReinstatedOnlyAfterCompletingAgain(t *testing.T) {
	rc := newRefundCertFixture(t)
	ctx := context.Background()
	rc.refund(rc.order)

	// Mua lại: ghi danh khôi phục với completed_at = nil (completeOrderFulfillment) -> chưa có chứng chỉ.
	rc.exec("UPDATE enrollments SET deleted_at = NULL, completed_at = NULL, progress_percentage = 0 WHERE user_id = ? AND course_id = ?", rc.student, rc.course)
	if list, err := rc.certSvc.GetMyCertificates(ctx, rc.student, 1, 10); err != nil || list.Total != 0 {
		t.Fatalf("mua lại nhưng chưa học xong: %+v err=%v, muốn 0 chứng chỉ", list, err)
	}

	// Học xong lần nữa -> cấp lại đúng số cũ, trang verify hợp lệ trở lại.
	rc.exec("UPDATE enrollments SET completed_at = now(), progress_percentage = 100 WHERE user_id = ? AND course_id = ?", rc.student, rc.course)
	list, err := rc.certSvc.GetMyCertificates(ctx, rc.student, 1, 10)
	if err != nil || list.Total != 1 || list.Data[0].CertificateNumber != rc.number {
		t.Fatalf("học xong lần hai: %+v err=%v, muốn 1 chứng chỉ số %s", list, err, rc.number)
	}
	if v, err := rc.certSvc.VerifyCertificate(ctx, rc.number); err != nil || !v.Valid || v.Revoked {
		t.Fatalf("verify sau khi cấp lại: %+v err=%v, muốn hợp lệ", v, err)
	}
}
