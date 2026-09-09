package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeOrderRepoForOwnership (H-06, audit 260909 vòng 2): fake tối giản chỉ implement
// GetByID — embed interface nil để mọi method khác panic nếu lỡ bị gọi. Không có sqlmock
// trong go.sum nên đây là cách test logic ownership mà không cần DB thật.
type fakeOrderRepoForOwnership struct {
	repository.OrderRepositoryInterface
	order *model.Order
	err   error
}

func (f *fakeOrderRepoForOwnership) GetByID(id uuid.UUID) (*model.Order, error) {
	return f.order, f.err
}

type fakeOrderItemRepoForOwnership struct {
	repository.OrderItemRepositoryInterface
	items []model.OrderItem
}

func (f *fakeOrderItemRepoForOwnership) GetByOrderID(orderID uuid.UUID) ([]model.OrderItem, error) {
	return f.items, nil
}

// TestGetOrderByID_Ownership (H-06) — trước đây GetOrderByID KHÔNG so order.UserID với actor
// gọi API, ai biết orderID cũng xem được đơn của người khác (IDOR).
func TestGetOrderByID_Ownership(t *testing.T) {
	ownerID := uuid.New()
	otherUserID := uuid.New()
	orderID := uuid.New()
	order := &model.Order{ID: orderID, UserID: ownerID, Status: "pending"}

	newSvc := func() *OrderService {
		return &OrderService{
			orderRepo:     &fakeOrderRepoForOwnership{order: order},
			orderItemRepo: &fakeOrderItemRepoForOwnership{},
		}
	}

	t.Run("chủ đơn hàng xem được", func(t *testing.T) {
		s := newSvc()
		if _, err := s.GetOrderByID(context.Background(), orderID, ownerID, false); err != nil {
			t.Errorf("expected owner access to succeed, got %v", err)
		}
	})

	t.Run("người khác bị từ chối (IDOR)", func(t *testing.T) {
		s := newSvc()
		_, err := s.GetOrderByID(context.Background(), orderID, otherUserID, false)
		if !errors.Is(err, ErrOrderForbidden) {
			t.Errorf("expected ErrOrderForbidden, got %v", err)
		}
	})

	t.Run("admin xem được đơn của người khác", func(t *testing.T) {
		s := newSvc()
		if _, err := s.GetOrderByID(context.Background(), orderID, otherUserID, true); err != nil {
			t.Errorf("expected admin access to succeed, got %v", err)
		}
	})
}

// TestCancelOrder_Ownership (H-06) — cùng nhánh IDOR cho CancelOrder; chỉ kiểm tra nhánh bị
// từ chối vì nhánh "được phép" đi tiếp vào WithTransaction (cần fake sâu hơn, ngoài phạm vi
// test ownership này — CancelOrder's transaction flow không phải mục tiêu của H-06 fix).
func TestCancelOrder_Ownership(t *testing.T) {
	ownerID := uuid.New()
	otherUserID := uuid.New()
	orderID := uuid.New()
	order := &model.Order{ID: orderID, UserID: ownerID, Status: "pending"}

	s := &OrderService{orderRepo: &fakeOrderRepoForOwnership{order: order}}

	err := s.CancelOrder(context.Background(), otherUserID, orderID, false, "khong thich nua")
	if !errors.Is(err, ErrOrderForbidden) {
		t.Errorf("expected ErrOrderForbidden for non-owner cancel, got %v", err)
	}
}
