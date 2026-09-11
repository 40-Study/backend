package model

import "testing"

// Pin quy ước VoucherUnlimitedUsage: 0 (và âm) = không giới hạn, chỉ > 0 mới là giới hạn thật.
// Đổi bất kỳ dấu so sánh nào trong 3 helper là test đỏ ngay.
func TestVoucherUsageHelpers_ZeroAndNegativeMeanUnlimited(t *testing.T) {
	for _, limit := range []int32{VoucherUnlimitedUsage, -1} {
		v := &Voucher{UsageLimit: limit, UsagePerUser: limit, UsedCount: 1_000_000}
		if v.HasUsageLimit() {
			t.Errorf("UsageLimit=%d: HasUsageLimit phải false", limit)
		}
		if v.HasPerUserLimit() {
			t.Errorf("UsagePerUser=%d: HasPerUserLimit phải false", limit)
		}
		if v.IsUsageLimitReached() {
			t.Errorf("UsageLimit=%d: IsUsageLimitReached phải false dù UsedCount rất lớn", limit)
		}
	}
}

func TestVoucherUsageHelpers_PositiveLimitIsReal(t *testing.T) {
	v := &Voucher{UsageLimit: 5, UsagePerUser: 1, UsedCount: 4}
	if !v.HasUsageLimit() || !v.HasPerUserLimit() {
		t.Fatal("giới hạn > 0 phải được coi là có giới hạn")
	}
	if v.IsUsageLimitReached() {
		t.Error("UsedCount=4 < UsageLimit=5: chưa hết lượt")
	}
	v.UsedCount = 5
	if !v.IsUsageLimitReached() {
		t.Error("UsedCount=5 >= UsageLimit=5: phải là hết lượt")
	}
}
