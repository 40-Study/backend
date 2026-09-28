package config

import (
	"testing"

	"github.com/shopspring/decimal"
)

// WITHDRAWAL_MIN_AMOUNT (quyết định #7): cấu hình được, mặc định 100.000đ, sai thì fail-fast.
func TestParseWithdrawalMinAmount(t *testing.T) {
	got, err := parseWithdrawalMinAmount(DefaultWithdrawalMinAmount)
	if err != nil || !got.Equal(decimal.NewFromInt(100000)) {
		t.Fatalf("mặc định = %s (err %v), muốn 100000", got, err)
	}
	if got, err := parseWithdrawalMinAmount(" 50000 "); err != nil || !got.Equal(decimal.NewFromInt(50000)) {
		t.Fatalf("50000 = %s (err %v)", got, err)
	}
	for _, bad := range []string{"", "abc", "0", "-100000"} {
		if _, err := parseWithdrawalMinAmount(bad); err == nil {
			t.Fatalf("%q phải bị từ chối", bad)
		}
	}
}
