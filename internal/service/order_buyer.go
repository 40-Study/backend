package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// orderBuyerOf đọc người mua của đơn cho trang chi tiết (QA4-A08). Đọc riêng thay vì Preload("User") trong
// OrderRepository.GetByID: các luồng thanh toán Save() đơn lấy từ GetByID, và một User được preload sẽ bị
// GORM ghi kèm theo. full_name trống thì dùng user_name để luôn có tên hiển thị. Tài khoản đã xoá mềm vẫn
// đọc được (Unscoped): lịch sử đơn phải còn biết ai đã mua.
func orderBuyerOf(ctx context.Context, db *gorm.DB, userID uuid.UUID) (*dto.OrderBuyer, error) {
	var u model.User
	err := db.WithContext(ctx).Unscoped().Select("id", "email", "full_name", "user_name").First(&u, "id = ?", userID).Error
	if err != nil {
		return nil, fmt.Errorf("read order buyer: %w", err)
	}
	name := u.UserName
	if u.FullName != nil && *u.FullName != "" {
		name = *u.FullName
	}
	return &dto.OrderBuyer{ID: u.ID, FullName: name, Email: u.Email}, nil
}
