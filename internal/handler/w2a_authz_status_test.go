package handler

// Lane W2-A: ánh xạ mã HTTP của các luật "không xem được thì 404, xem được mà không có quyền thì 403".
//   - đơn: id không tồn tại và đơn của người khác phải cùng 404, cùng thân JSON trên MỌI route đơn/thanh toán;
//   - thông báo: xoá/đánh dấu đã đọc thông báo không phải của mình hoặc không có là 404;
//   - bài nộp: không xem được bài tập là 404, xem được mà không phải chủ/người quản lý là 403;
//   - lớp: hai lỗi nghiệp vụ mới mang mã ổn định để web dịch (TEACHER_NOT_ORG_MEMBER 422, CLASS_DELETE_ADMIN_ONLY 403).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type w2aOrderSvc struct {
	service.OrderServiceInterface
	err error
}

func (f w2aOrderSvc) GetOrderByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.OrderResponse, error) {
	return nil, f.err
}
func (f w2aOrderSvc) CancelOrder(context.Context, uuid.UUID, uuid.UUID, bool, string) error {
	return f.err
}

type w2aPaymentSvc struct {
	service.PaymentServiceInterface
	err error
}

func (f w2aPaymentSvc) CreatePaymentIntent(context.Context, uuid.UUID, uuid.UUID, bool, string) (*dto.PaymentIntentResponse, error) {
	return nil, f.err
}
func (f w2aPaymentSvc) CheckAndProcessPayment(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.PaymentStatusResponse, error) {
	return nil, f.err
}
func (f w2aPaymentSvc) GetPaymentStatus(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.PaymentStatusResponse, error) {
	return nil, f.err
}

func TestW2A_Order_IdKhongCoVaDonNguoiKhacCungMotMaLoi(t *testing.T) {
	routes := []struct{ method, path string }{
		{"GET", ""}, {"POST", "/cancel"}, {"POST", "/payment-intent"}, {"GET", "/payment-status"}, {"POST", "/check-payment"},
	}
	bodyOf := func(err error, r struct{ method, path string }) (int, string) {
		h := NewOrderHandler(w2aOrderSvc{err: err}, w2aPaymentSvc{err: err}, nil)
		app := fiber.New()
		with := func(f fiber.Handler) fiber.Handler {
			return func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return f(c) }
		}
		app.Get("/orders/:id", with(h.GetOrder))
		app.Post("/orders/:id/cancel", with(h.CancelOrder))
		app.Post("/orders/:id/payment-intent", with(h.CreatePaymentIntent))
		app.Get("/orders/:id/payment-status", with(h.GetPaymentStatus))
		app.Post("/orders/:id/check-payment", with(h.CheckPayment))
		req := httptest.NewRequest(r.method, "/orders/"+uuid.NewString()+r.path, strings.NewReader(`{"payment_method":"qr_transfer"}`))
		req.Header.Set("Content-Type", "application/json")
		res, e := app.Test(req, -1)
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(raw)
	}
	for _, r := range routes {
		notFoundStatus, notFoundBody := bodyOf(service.ErrOrderNotFound, r)
		otherStatus, otherBody := bodyOf(service.ErrOrderForbidden, r)
		if notFoundStatus != fiber.StatusNotFound || otherStatus != fiber.StatusNotFound {
			t.Errorf("%s %s: id không có=%d, đơn người khác=%d, muốn cùng 404", r.method, r.path, notFoundStatus, otherStatus)
		}
		if notFoundBody != otherBody {
			t.Errorf("%s %s: hai trường hợp khác thân JSON (%s vs %s): người lạ phân biệt được", r.method, r.path, notFoundBody, otherBody)
		}
	}
}

type w2aNotificationSvc struct {
	service.NotificationServiceInterface
	err error
}

func (f w2aNotificationSvc) DeleteNotification(uuid.UUID, uuid.UUID) error { return f.err }
func (f w2aNotificationSvc) MarkAsRead(uuid.UUID, uuid.UUID) error         { return f.err }

func TestW2A_Notification_KhongCoHoacCuaNguoiKhacLa404(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"không có hoặc của người khác", service.ErrNotificationNotFound, fiber.StatusNotFound},
		{"của chính mình", nil, fiber.StatusOK},
	} {
		h := NewNotificationHandler(w2aNotificationSvc{err: tc.err})
		app := fiber.New()
		with := func(f fiber.Handler) fiber.Handler {
			return func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return f(c) }
		}
		app.Delete("/notifications/:id", with(h.DeleteNotification))
		app.Patch("/notifications/:id/read", with(h.MarkAsRead))
		for _, r := range []struct{ method, path string }{{"DELETE", ""}, {"PATCH", "/read"}} {
			res, err := app.Test(httptest.NewRequest(r.method, "/notifications/"+uuid.NewString()+r.path, nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.status {
				t.Errorf("%s %s (%s): status=%d, muốn %d", r.method, r.path, tc.name, res.StatusCode, tc.status)
			}
		}
	}
}

type w2aSubmissionSvc struct {
	service.SubmissionServiceInterface
	err error
}

func (f w2aSubmissionSvc) GetByAssignment(context.Context, uuid.UUID, uuid.UUID, bool, int, int) (*dto.SubmissionListDTO, error) {
	return nil, f.err
}
func (f w2aSubmissionSvc) GetByID(context.Context, uuid.UUID, uuid.UUID, bool) (*model.Submission, error) {
	return nil, f.err
}

func TestW2A_Submission_KhongXemDuocBaiTap404_XemDuocKhongQuyen403(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{service.ErrAssignmentNotFound, fiber.StatusNotFound},
		{service.ErrSubmissionForbidden, fiber.StatusForbidden},
	} {
		h := NewSubmissionHandler(w2aSubmissionSvc{err: tc.err}, nil)
		app := fiber.New()
		with := func(f fiber.Handler) fiber.Handler {
			return func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return f(c) }
		}
		app.Get("/submissions/assignment/:assignmentId", with(h.GetByAssignment))
		app.Get("/submissions/:id", with(h.GetByID))
		for _, path := range []string{"/submissions/assignment/" + uuid.NewString(), "/submissions/" + uuid.NewString()} {
			res, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.status {
				t.Errorf("GET %s với lỗi %v: status=%d, muốn %d", path, tc.err, res.StatusCode, tc.status)
			}
		}
	}
}

func TestW2A_ClassErrorBody_MaOnDinh(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{service.ErrTeacherNotOrgMember, fiber.StatusBadRequest, "TEACHER_NOT_ORG_MEMBER"},
		{service.ErrClassDeleteAdminOnly, fiber.StatusForbidden, "CLASS_DELETE_ADMIN_ONLY"},
	} {
		wrapped := errors.Join(errors.New("gán giảng viên"), tc.err)
		if got := classErrorStatus(wrapped); got != tc.status {
			t.Errorf("classErrorStatus(%v)=%d, muốn %d", tc.err, got, tc.status)
		}
		raw, _ := json.Marshal(classErrorBody(wrapped))
		if !strings.Contains(string(raw), `"code":"`+tc.code+`"`) {
			t.Errorf("thân lỗi %s thiếu code %s", raw, tc.code)
		}
	}
}
