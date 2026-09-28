package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Test thuần cho công thức + quy tắc rút tiền (chạy được cả khi không có Postgres).

func d(v int64) decimal.Decimal { return decimal.NewFromInt(v) }

func TestAvailableBalance_ExcludesRejectedAndCanBeNegative(t *testing.T) {
	sums := repository.TeacherPayoutSums{Open: d(100000), Completed: d(200000)}
	if got := availableBalance(d(1000000), sums); !got.Equal(d(700000)) {
		t.Fatalf("availableBalance = %s, muốn 700000", got)
	}
	// Hoàn tiền sau khi đã rút -> thu nhập < đã rút -> âm, không bị kẹp về 0.
	if got := availableBalance(d(100000), sums); !got.Equal(d(-200000)) {
		t.Fatalf("availableBalance = %s, muốn -200000", got)
	}
}

func TestValidateWithdrawalAmount(t *testing.T) {
	min := d(100000)
	cases := []struct {
		amount int64
		want   error
	}{
		{0, ErrWithdrawalInvalidAmount},
		{-5, ErrWithdrawalInvalidAmount},
		{99999, ErrWithdrawalBelowMinimum},
		{100000, nil},
		{250000, nil},
	}
	for _, c := range cases {
		err := validateWithdrawalAmount(d(c.amount), min)
		if !errors.Is(err, c.want) && !(err == nil && c.want == nil) {
			t.Fatalf("amount %d: err = %v, muốn %v", c.amount, err, c.want)
		}
	}
}

func TestCheckWithdrawalEligibility(t *testing.T) {
	bank, num, name, blank := "VCB", "0123", "NGUYEN VAN A", "  "
	withBank := &model.TeacherProfile{BankName: &bank, BankAccountNumber: &num, BankAccountName: &name}
	blankName := &model.TeacherProfile{BankName: &bank, BankAccountNumber: &num, BankAccountName: &blank}
	openID := uuid.New()

	cases := []struct {
		name    string
		profile *model.TeacherProfile
		open    *uuid.UUID
		balance int64
		amount  int64
		want    error
	}{
		{"thiếu bank info", &model.TeacherProfile{}, nil, 500000, 100000, ErrWithdrawalBankInfoRequired},
		{"bank info chỉ có khoảng trắng", blankName, nil, 500000, 100000, ErrWithdrawalBankInfoRequired},
		{"đang có yêu cầu mở", withBank, &openID, 500000, 100000, ErrWithdrawalAlreadyOpen},
		{"số dư âm", withBank, nil, -1, 100000, ErrWithdrawalNegativeBalance},
		{"số dư 0", withBank, nil, 0, 100000, ErrWithdrawalInsufficientBalance},
		{"vượt số dư", withBank, nil, 150000, 200000, ErrWithdrawalInsufficientBalance},
		{"đúng bằng số dư", withBank, nil, 200000, 200000, nil},
	}
	for _, c := range cases {
		err := checkWithdrawalEligibility(c.profile, c.open, d(c.balance), d(c.amount))
		if c.want == nil {
			if err != nil {
				t.Fatalf("%s: err = %v, muốn nil", c.name, err)
			}
			continue
		}
		if !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, muốn %v", c.name, err, c.want)
		}
	}
}
