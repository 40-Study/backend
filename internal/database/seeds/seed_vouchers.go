package seeds

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

type VoucherSeed struct {
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	DiscountUnit       string  `json:"discount_unit"`
	DiscountMethod     string  `json:"discount_method"`
	DiscountPercent    float64 `json:"discount_percent"`
	DiscountAmountMoney float64 `json:"discount_amount_money"`
	MaxDiscountMoney   float64 `json:"max_discount_money"`
	MinPurchaseMoney   float64 `json:"min_purchase_money"`
	UsageLimit         int32   `json:"usage_limit"`
	IsActive           bool    `json:"is_active"`
}

func (s *Seeder) SeedVouchers(filePath string) error {
	log.Println("Seeding vouchers...")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read vouchers file: %w", err)
	}

	var seeds []VoucherSeed
	if err := json.Unmarshal(data, &seeds); err != nil {
		return fmt.Errorf("failed to parse vouchers JSON: %w", err)
	}

	for _, vs := range seeds {
		voucher := model.Voucher{
			Code:           vs.Code,
			Name:           vs.Name,
			Description:    vs.Description,
			DiscountUnit:   model.DiscountUnit(vs.DiscountUnit),
			DiscountMethod: model.DiscountMethod(vs.DiscountMethod),
			UsageLimit:     vs.UsageLimit,
			IsActive:       vs.IsActive,
			CanStack:       false,
			UsagePerUser:   1,
			AcceptAllPaymentMethods: true,
		}

		if vs.DiscountMethod == string(model.DiscountMethodPercent) && vs.DiscountPercent > 0 {
			p := decimal.NewFromFloat(vs.DiscountPercent)
			voucher.DiscountPercent = &p
		}

		if vs.DiscountMethod == string(model.DiscountMethodFixed) && vs.DiscountAmountMoney > 0 {
			a := decimal.NewFromFloat(vs.DiscountAmountMoney)
			voucher.DiscountAmountMoney = &a
		}

		if vs.MaxDiscountMoney > 0 {
			m := decimal.NewFromFloat(vs.MaxDiscountMoney)
			voucher.MaxDiscountMoney = &m
		}

		if vs.MinPurchaseMoney > 0 {
			mp := decimal.NewFromFloat(vs.MinPurchaseMoney)
			voucher.MinPurchaseMoney = &mp
		}

		result := s.db.Where("code = ?", vs.Code).FirstOrCreate(&voucher)
		if result.Error != nil {
			return fmt.Errorf("failed to seed voucher %s: %w", vs.Code, result.Error)
		}

		log.Printf("Seeded voucher: %s\n", vs.Code)
	}

	log.Printf("Successfully seeded %d vouchers\n", len(seeds))
	return nil
}
