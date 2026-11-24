package service

// QA4-A08 (contract 5): trang chi tiết đơn của admin không có người mua vì GET /orders/:id không trả trường nào
// về user. Giờ response có buyer { id, full_name, email }: admin thấy người mua, chủ đơn thấy chính mình.
// Postgres THẬT (orderFixture). Mutation đã kiểm: bỏ gán resp.Buyer làm test ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func TestGetOrderByID_IncludesBuyer(t *testing.T) {
	f := newOrderFixture(t)
	buyer := f.user()
	if err := f.db.Model(&model.User{}).Where("id = ?", buyer).Update("full_name", "Nguyễn Thị Mua").Error; err != nil {
		t.Fatal(err)
	}
	var email string
	if err := f.db.Model(&model.User{}).Where("id = ?", buyer).Pluck("email", &email).Error; err != nil {
		t.Fatal(err)
	}
	order := f.createOrder(buyer, f.course("Khoá có người mua"))

	for _, tc := range []struct {
		name    string
		actor   uuid.UUID
		isAdmin bool
	}{{"admin", f.user(), true}, {"chủ đơn", buyer, false}} {
		got, err := f.svc.GetOrderByID(context.Background(), order.ID, tc.actor, tc.isAdmin)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		b := got.Buyer
		if b == nil || b.ID != buyer || b.FullName != "Nguyễn Thị Mua" || b.Email != email {
			t.Fatalf("%s: buyer = %+v, muốn %s / Nguyễn Thị Mua / %s", tc.name, b, buyer, email)
		}
	}
}
