package service

// Re-review vòng 2 PR #79 (D4): khoá chưa xuất bản (nháp/chờ duyệt/bị từ chối) coi như không tồn
// tại với các đường TỰ ghi danh / thêm giỏ / tạo đơn. Trước bản vá, student tự ghi danh khoá nháp
// giá 0 được (201), rồi canViewCourse tin cờ enrolled nên đọc được toàn bộ nội dung khoá nháp.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// errReachedRepo — fake repo trả lỗi này khi luồng đi QUA được cổng trạng thái khoá (đối chứng:
// khoá đã xuất bản không bị chặn nhầm).
var errReachedRepo = errors.New("reached repo after course gate")

type privCourseRepo struct {
	repository.CourseRepositoryInterface
	courses map[uuid.UUID]*model.Course
}

func (r *privCourseRepo) GetByID(_ context.Context, id uuid.UUID) (*model.Course, error) {
	return r.courses[id], nil
}

type privEnrollmentRepo struct {
	repository.EnrollmentRepositoryInterface
}

func (privEnrollmentRepo) GetByUserAndCourseUnscoped(context.Context, uuid.UUID, uuid.UUID) (*model.Enrollment, error) {
	return nil, errReachedRepo
}

type privCartRepo struct {
	repository.CartItemRepositoryInterface
}

func (privCartRepo) Exists(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, errReachedRepo
}

type privOrderRepo struct {
	repository.OrderRepositoryInterface
}

func (privOrderRepo) GetExpiredHeldOrdersForUser(uuid.UUID, time.Duration) ([]model.Order, error) {
	return nil, nil
}

func privCourses() (*privCourseRepo, []uuid.UUID, uuid.UUID) {
	repo := &privCourseRepo{courses: map[uuid.UUID]*model.Course{}}
	var private []uuid.UUID
	for _, st := range []string{model.CourseStatusDraft, model.CourseStatusPendingReview, model.CourseStatusRejected} {
		id := uuid.New()
		repo.courses[id] = &model.Course{BaseModel: model.BaseModel{ID: id}, Status: st, InstructorID: uuid.New()}
		private = append(private, id)
	}
	published := uuid.New()
	repo.courses[published] = &model.Course{BaseModel: model.BaseModel{ID: published}, Status: model.CourseStatusPublished, InstructorID: uuid.New()}
	return repo, private, published
}

func TestEnroll_PrivateCourseIsNotFound(t *testing.T) {
	courses, private, published := privCourses()
	svc := NewEnrollmentService(privEnrollmentRepo{}, courses, nil, nil)
	for _, id := range append(private, uuid.New()) { // + khoá không tồn tại: CÙNG lỗi
		if _, err := svc.Enroll(context.Background(), uuid.New(), id); !errors.Is(err, ErrCourseHidden) {
			t.Errorf("Enroll khoá %q: muốn ErrCourseHidden (404), nhận %v", statusOf(courses, id), err)
		}
	}
	if _, err := svc.Enroll(context.Background(), uuid.New(), published); !errors.Is(err, errReachedRepo) {
		t.Errorf("khoá published miễn phí bị chặn nhầm: %v", err)
	}
}

func TestAddToCart_PrivateCourseIsNotFound(t *testing.T) {
	courses, private, published := privCourses()
	svc := NewCartService(privCartRepo{}, courses, privEnrollmentRepo{})
	for _, id := range private {
		_, err := svc.AddToCart(context.Background(), uuid.New(), dto.AddToCartDTO{CourseID: id})
		var known *apperr.KnownError
		if !errors.As(err, &known) || known.Status != http.StatusNotFound {
			t.Errorf("AddToCart khoá %q: muốn 404, nhận %v", statusOf(courses, id), err)
		}
	}
	if _, err := svc.AddToCart(context.Background(), uuid.New(), dto.AddToCartDTO{CourseID: published}); !errors.Is(err, errReachedRepo) {
		t.Errorf("khoá published bị chặn nhầm: %v", err)
	}
}

func TestCreateOrder_PrivateCourseIsNotFound(t *testing.T) {
	courses, private, _ := privCourses()
	svc := NewOrderService(privOrderRepo{}, nil, courses, nil, nil, nil)
	for _, id := range private {
		req := dto.CreateOrderRequest{Source: "buy_now", CourseIDs: []string{id.String()}}
		if _, err := svc.CreateOrder(context.Background(), uuid.New(), req); !errors.Is(err, ErrCourseNotFound) {
			t.Errorf("CreateOrder khoá %q: muốn ErrCourseNotFound, nhận %v", statusOf(courses, id), err)
		}
	}
}

func statusOf(r *privCourseRepo, id uuid.UUID) string {
	if c := r.courses[id]; c != nil {
		return c.Status
	}
	return "không tồn tại"
}
