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
			Code:                    "WELCOME20",
			Name:                    "Chào mừng học viên mới - giảm 20%",
			Description:             "Giảm 20% cho đơn hàng đầu tiên, tối đa 200.000đ.",
			DiscountUnit:            model.DiscountUnitMoney,
			DiscountMethod:          model.DiscountMethodPercent,
			DiscountPercent:         ptr(rating(20)),
			MaxDiscountMoney:        ptr(money(200000)),
			MinPurchaseMoney:        ptr(money(0)),
			AcceptAllPaymentMethods: true,
			UsageLimit:              1000,
			UsagePerUser:            1,
			StartDate:               &start,
			EndDate:                 &end,
			IsActive:                true,
		},
		{
			Code:                    "SUMMER50K",
			Name:                    "Ưu đãi hè - giảm 50.000đ",
			Description:             "Giảm thẳng 50.000đ cho đơn từ 300.000đ.",
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
			Name:                    "Ưu đãi VIP - giảm 100.000đ",
			Description:             "Giảm 100.000đ cho đơn từ 500.000đ, số lượng giới hạn.",
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
		if err := s.db.Where("code = ?", v.Code).
			Attrs(record).
			FirstOrCreate(&record).Error; err != nil {
			return fmt.Errorf("failed to seed voucher %s: %w", v.Code, err)
		}
	}

	log.Printf("Seeded %d vouchers\n", len(vouchers))
	return nil
}
