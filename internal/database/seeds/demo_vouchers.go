package seeds

import (
	"fmt"
	"log"

	"study.com/v1/internal/model"
)

// SeedDemoVouchers tạo mã giảm giá demo cho luồng giỏ hàng / thanh toán.
func (s *Seeder) SeedDemoVouchers() error {
	log.Println("Seeding demo vouchers...")

	start := daysAgo(10)
	end := daysAhead(90)

	vouchers := []model.Voucher{
		{
			Code:                    model.SeededFirstOrderVoucherCode,
			Name:                    "Chào mừng học viên mới - giảm 20%",
			Description:             "Giảm 20% cho đơn hàng đầu tiên, tối đa 200.000 ₫.",
			DiscountUnit:            model.DiscountUnitMoney,
			DiscountMethod:          model.DiscountMethodPercent,
			DiscountPercent:         ptr(rating(20)),
			MaxDiscountMoney:        ptr(money(200000)),
			MinPurchaseMoney:        ptr(money(0)),
			AcceptAllPaymentMethods: true,
			UsageLimit:              1000,
			UsagePerUser:            1,
			FirstOrderOnly:          true, // mô tả "đơn hàng đầu tiên" là luật thật (QA2 F-02), không chỉ chữ
			StartDate:               &start,
			EndDate:                 &end,
			IsActive:                true,
		},
		{
			Code:                    "SUMMER50K",
			Name:                    "Ưu đãi hè - giảm 50.000 ₫",
			Description:             "Giảm thẳng 50.000 ₫ cho đơn từ 300.000 ₫.",
			DiscountUnit:            model.DiscountUnitMoney,
			DiscountMethod:          model.DiscountMethodFixed,
			DiscountAmountMoney:     ptr(money(50000)),
			MinPurchaseMoney:        ptr(money(300000)),
			AcceptAllPaymentMethods: true,
			UsageLimit:              500,
			UsagePerUser:            1,
			StartDate:               &start,
			EndDate:                 &end,
			IsActive:                true,
		},
		{
			Code:                    "VIP100",
			Name:                    "Ưu đãi VIP - giảm 100.000 ₫",
			Description:             "Giảm 100.000 ₫ cho đơn từ 500.000 ₫, số lượng giới hạn.",
			DiscountUnit:            model.DiscountUnitMoney,
			DiscountMethod:          model.DiscountMethodFixed,
			DiscountAmountMoney:     ptr(money(100000)),
			MinPurchaseMoney:        ptr(money(500000)),
			AcceptAllPaymentMethods: true,
			UsageLimit:              100,
			UsagePerUser:            1,
			StartDate:               &start,
			EndDate:                 &end,
			IsActive:                true,
		},
	}

	for _, v := range vouchers {
		record := v
		// Assign (QA5 S5-15): DB đã seed bằng chữ cũ ("50.000đ") được sửa tên/mô tả cho cùng kiểu "50.000 ₫".
		if err := s.db.Where("code = ?", v.Code).
			Attrs(record).
			Assign(map[string]interface{}{"name": v.Name, "description": v.Description}).
			FirstOrCreate(&record).Error; err != nil {
			return fmt.Errorf("failed to seed voucher %s: %w", v.Code, err)
		}
		// FirstOrCreate không chạm dòng đã có: DB demo seed trước QA2 F-02 vẫn thiếu cờ first_order_only.
		if v.FirstOrderOnly && !record.FirstOrderOnly {
			if err := s.db.Model(&model.Voucher{}).Where("id = ?", record.ID).
				Update("first_order_only", true).Error; err != nil {
				return fmt.Errorf("failed to flag voucher %s first_order_only: %w", v.Code, err)
			}
		}
	}

	log.Printf("Seeded %d vouchers\n", len(vouchers))
	return nil
}
