package service

// QA4-S7: dịch vụ ngân hàng không trả lời (gRPC "connection refused") làm VerifyPurchase trả nguyên lỗi hạ
// tầng cho người dùng. Giờ phải trả ErrCoinBankUnavailable (handler -> 503 ERR_PAYMENT_PROVIDER_UNAVAILABLE),
// không lộ chuỗi lỗi gốc, không cộng xu. Postgres THẬT như coin_purchase_checkout_test.go.
// Mutation đã kiểm: trả lại fmt.Errorf("failed to check transaction: %w", err) làm test ĐỎ.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestCoinVerify_BankUnavailable_NoRawError(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	user := f.user("coin-bank-down")
	pkg := model.CoinPackage{Name: "QA4-S7", CoinAmount: 50, Price: decimal.NewFromInt(10000), Currency: "VND", IsActive: true}
	if err := f.db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	raw := errors.New("rpc error: code = Unavailable desc = dial tcp [::1]:50051: connect: connection refused")
	newSvc := func(bank TransactionServiceInterface) *CoinService {
		return NewCoinService(repository.NewCoinWalletRepository(f.db), repository.NewCoinTransactionRepository(f.db),
			repository.NewCoinPackageRepository(f.db), repository.NewCoinPurchaseRepository(f.db), bank)
	}
	svc := newSvc(&fakeBankLookup{err: raw})
	p, err := svc.CreatePurchase(ctx, user, dto.CreateCoinPurchaseRequest{PackageID: pkg.ID, PaymentMethod: "bank_transfer"})
	if err != nil {
		t.Fatal(err)
	}

	for name, s := range map[string]*CoinService{"ngân hàng lỗi": svc, "chưa cấu hình": newSvc(nil)} {
		_, err = s.VerifyPurchase(ctx, user, p.ID)
		if !errors.Is(err, ErrCoinBankUnavailable) {
			t.Fatalf("%s: err = %v, muốn ErrCoinBankUnavailable", name, err)
		}
		if strings.Contains(err.Error(), "rpc") || strings.Contains(err.Error(), "50051") {
			t.Fatalf("%s: lộ lỗi hạ tầng: %q", name, err)
		}
	}
	var purchase model.CoinPurchase
	if err := f.db.First(&purchase, "id = ?", p.ID).Error; err != nil {
		t.Fatal(err)
	}
	if purchase.Status != model.CoinPurchasePending {
		t.Fatalf("status = %s, muốn vẫn pending", purchase.Status)
	}
}
