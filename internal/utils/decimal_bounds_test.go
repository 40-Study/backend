package utils

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// DecimalInClosedRange phải loại số mũ khổng lồ NGAY (O(1)), không rescale 10^|exp|. Bỏ DecimalExponentWithinBounds khỏi hàm thì
// ca "mũ lớn" tốn ~2 s mỗi lần (1e20000000) và vượt trần 500 ms.
func TestDecimalInClosedRange(t *testing.T) {
	lo, hi := decimal.Zero, decimal.NewFromInt(100)
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"0", true}, {"100", true}, {"55.5", true}, {"100.000", true},
		{"-0.01", false}, {"100.01", false}, {"1e3", false},
		{"1e20000000", false}, {"1e-20000000", false}, {"0e20000000", false},
	} {
		start := time.Now()
		got := DecimalInClosedRange(decimal.RequireFromString(tc.in), lo, hi)
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Errorf("%s: mất %v, muốn loại ngay trước khi so sánh", tc.in, elapsed)
		}
		if got != tc.want {
			t.Errorf("DecimalInClosedRange(%s, 0, 100) = %v, muốn %v", tc.in, got, tc.want)
		}
	}
}
