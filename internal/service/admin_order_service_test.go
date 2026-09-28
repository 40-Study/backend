package service

import "testing"

// TestCalculateSuccessRate (phase-02 contract) — pin công thức success_rate: loại pending/
// processing khỏi mẫu số, KHÔNG đánh đồng cancelled với refunded.
func TestCalculateSuccessRate(t *testing.T) {
	cases := []struct {
		name                                  string
		completed, failed, cancelled, expired int64
		want                                  float64
	}{
		{"tat ca hoan tat", 4, 0, 0, 0, 100},
		{"nua thanh cong", 2, 2, 0, 0, 50},
		{"mau so rong khong chia cho 0", 0, 0, 0, 0, 0},
		{"cancelled tinh vao mau so nhung khong phai tu so", 3, 0, 1, 0, 75},
		{"expired tinh vao mau so", 1, 0, 0, 1, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateSuccessRate(tc.completed, tc.failed, tc.cancelled, tc.expired)
			if got != tc.want {
				t.Fatalf("calculateSuccessRate(%d,%d,%d,%d) = %v, want %v",
					tc.completed, tc.failed, tc.cancelled, tc.expired, got, tc.want)
			}
		})
	}
}

// TestCalculateSuccessRate_RefundedNotInDenominator: refunded KHÔNG truyền vào hàm (chỉ 4 tham
// số completed/failed/cancelled/expired) — đơn refunded từng "completed" thật, tính riêng ở
// refund_amount, không được lẫn vào mẫu số success_rate của kỳ báo cáo (nếu lẫn vào, 1 đơn vừa
// completed vừa refunded trong cùng kỳ sẽ bị đếm 2 lần ở mẫu số, sai công thức phase-02).
func TestCalculateSuccessRate_RefundedNotInDenominator(t *testing.T) {
	// 2 đơn completed (1 trong số đó SAU ĐÓ bị refund — nhưng completed_count SQL đếm theo
	// status HIỆN TẠI, nên đơn đã refund không còn nằm trong completed_count nữa) + 0 thất bại.
	got := calculateSuccessRate(1, 0, 0, 0)
	if got != 100 {
		t.Fatalf("got %v, want 100 (refunded rơi khỏi completed_count, không kéo success_rate xuống)", got)
	}
}
