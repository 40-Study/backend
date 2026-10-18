package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"sync"
	"testing"
	"time"
)

func TestCoinCheckoutResumeAndCreditOnce(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	user := f.user("coin-checkout")
	other := f.user("coin-other")
	pkg := model.CoinPackage{Name: "QA", CoinAmount: 100, BonusAmount: 20, Price: decimal.NewFromInt(20000), Currency: "VND", IsActive: true}
	if e := f.db.Create(&pkg).Error; e != nil {
		t.Fatal(e)
	}
	bank := &fakeBankLookup{result: &grpc.CheckTransactionResult{Found: false}}
	s := NewCoinService(repository.NewCoinWalletRepository(f.db), repository.NewCoinTransactionRepository(f.db), repository.NewCoinPackageRepository(f.db), repository.NewCoinPurchaseRepository(f.db), bank)
	req := dto.CreateCoinPurchaseRequest{PackageID: pkg.ID, PaymentMethod: "bank_transfer"}
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := s.CreatePurchase(ctx, user, req)
			if e != nil {
				errs <- e
				return
			}
			ids <- p.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	var id uuid.UUID
	for got := range ids {
		if id != uuid.Nil && id != got {
			t.Fatal("parallel creates issued two references")
		}
		id = got
	}
	p, e := s.VerifyPurchase(ctx, user, id)
	if e != nil || p.Status != "PENDING" {
		t.Fatalf("pending=%+v error=%v", p, e)
	}
	if _, e = s.VerifyPurchase(ctx, other, id); e == nil {
		t.Fatal("other user verified purchase")
	}
	bank.result = &grpc.CheckTransactionResult{Found: true, TransactionID: "coin-qa-" + id.String(), Amount: "19999", Currency: "VND", TransactionDate: time.Now().Format(time.RFC3339)}
	if _, e = s.VerifyPurchase(ctx, user, id); e == nil {
		t.Fatal("wrong amount accepted")
	}
	bank.result.Amount = "20000"
	p, e = s.VerifyPurchase(ctx, user, id)
	if e != nil || p.Status != "COMPLETED" {
		t.Fatalf("complete=%+v error=%v", p, e)
	}
	if _, e = s.VerifyPurchase(ctx, user, id); e != nil {
		t.Fatal(e)
	}
	var wallet model.UserCoinWallet
	if e = f.db.First(&wallet, "user_id = ?", user).Error; e != nil {
		t.Fatal(e)
	}
	if wallet.Balance != 120 {
		t.Fatalf("balance=%d", wallet.Balance)
	}
	var count int64
	f.db.Model(&model.CoinTransaction{}).Where("reference_id = ?", id).Count(&count)
	if count != 1 {
		t.Fatalf("ledger count=%d", count)
	}
	next, e := s.CreatePurchase(ctx, user, req)
	if e != nil || next.ID == id {
		t.Fatalf("new purchase=%+v error=%v", next, e)
	}
}
func TestBankDetailsNeverInventAccount(t *testing.T) {
	t.Setenv("MB_BANK_NAME", "")
	t.Setenv("MB_ACCOUNT_NO", "")
	t.Setenv("BANK_ACCOUNT_NAME", "")
	b, a, n := getBankTransferInfoFromEnv()
	if b != "" || a != "" || n != "" {
		t.Fatal("invented bank account")
	}
}
