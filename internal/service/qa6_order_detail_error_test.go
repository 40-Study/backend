package service

// QA6-A06: GetOrderByID chỉ trả ErrOrderNotFound khi repo báo "không có dòng"; lỗi DB phải lộ ra nguyên vẹn (admin trả 500).
// Trước đây mọi lỗi repo đều thành ErrOrderNotFound. Đổi lại `return nil, ErrOrderNotFound` vô điều kiện thì ca DB ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type qa6FailingOrderRepo struct {
	repository.OrderRepositoryInterface
	err error
}

func (r qa6FailingOrderRepo) GetByID(uuid.UUID) (*model.Order, error) { return nil, r.err }

func TestQA6_GetOrderByID_LoiDBKhongThanhKhongTimThay(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := &OrderService{orderRepo: qa6FailingOrderRepo{err: dbErr}}
	if _, err := svc.GetOrderByID(context.Background(), uuid.New(), uuid.Nil, true); !errors.Is(err, dbErr) || errors.Is(err, ErrOrderNotFound) {
		t.Errorf("lỗi DB: err=%v, muốn giữ nguyên lỗi DB (không phải ErrOrderNotFound)", err)
	}
	svc = &OrderService{orderRepo: qa6FailingOrderRepo{err: repository.ErrOrderNotFound}}
	if _, err := svc.GetOrderByID(context.Background(), uuid.New(), uuid.Nil, true); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("không có dòng: err=%v, muốn ErrOrderNotFound", err)
	}
}
