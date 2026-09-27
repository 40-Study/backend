package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

// TestCalculatePlatformFeeAmount (quyết định #2, 27/09/2026): % phí nền tảng CHỐT vào đơn —
// pin công thức totalAmount * percent / 100, làm tròn 2 chữ số. Đặc biệt: percent=0 (mặc định)
// luôn cho fee=0 bất kể totalAmount, kể cả trên đơn 0đ (totalAmount=0) — cả 2 trường hợp phải
// KHÔNG BAO GIỜ ra số âm hay NaN.
func TestCalculatePlatformFeeAmount(t *testing.T) {
	cases := []struct {
		name        string
		totalAmount string
		feePercent  string
		want        string
	}{
		{"phi 0% mac dinh", "1000000", "0", "0"},
		{"phi 5%", "1000000", "5", "50000"},
		{"phi 10% lam tron 2 chu so", "333333", "10", "33333.30"},
		{"don 0d bat ky percent nao cung ra fee 0", "0", "20", "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			total, _ := decimal.NewFromString(tc.totalAmount)
			percent, _ := decimal.NewFromString(tc.feePercent)
			want, _ := decimal.NewFromString(tc.want)
			got := calculatePlatformFeeAmount(total, percent)
			if !got.Equal(want) {
				t.Fatalf("calculatePlatformFeeAmount(%s, %s) = %s, want %s", tc.totalAmount, tc.feePercent, got, want)
			}
		})
	}
}

// TestSetPlatformFeePercent_UnchangedOrdersAfterConfigChange (quyết định #2: "đổi % sau không
// ảnh hưởng đơn cũ") — pin RÕ tính chất "chốt": gọi calculatePlatformFeeAmount với % CŨ và % MỚI
// trên CÙNG totalAmount phải cho 2 kết quả khác nhau, chứng minh giá trị phụ thuộc % TRUYỀN VÀO
// tại thời điểm gọi (snapshot), không đọc lại cấu hình hiện hành — hàm thuần này chính là điểm
// chốt: PaymentService.CheckAndProcessPayment gọi nó ĐÚNG MỘT LẦN lúc hoàn tất, kết quả được ghi
// cứng vào cột platform_fee_amount, không tính lại sau đó.
func TestSetPlatformFeePercent_UnchangedOrdersAfterConfigChange(t *testing.T) {
	total := decimal.NewFromInt(1000000)
	oldPercent := decimal.NewFromInt(5)
	newPercent := decimal.NewFromInt(10)

	feeAtOldConfig := calculatePlatformFeeAmount(total, oldPercent)
	feeAtNewConfig := calculatePlatformFeeAmount(total, newPercent)

	if feeAtOldConfig.Equal(feeAtNewConfig) {
		t.Fatalf("fee tai ty le cu (%s) va ty le moi (%s) phai KHAC nhau tren cung total_amount — neu bang nhau, chung minh ham dang doc lai cau hinh thay vi dung gia tri da chot",
			feeAtOldConfig, feeAtNewConfig)
	}
	if !feeAtOldConfig.Equal(decimal.NewFromInt(50000)) {
		t.Fatalf("fee tai %% cu = %s, want 50000 (5%% cua 1,000,000)", feeAtOldConfig)
	}
}
