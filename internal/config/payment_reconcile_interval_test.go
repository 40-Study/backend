package config

import "testing"

// PAYMENT_RECONCILE_INTERVAL_MINUTES (L6 mục 5): mặc định 10, hợp lệ 1-59, ngoài khoảng thì lỗi.
func TestValidatePaymentReconcileInterval(t *testing.T) {
	if got, err := validatePaymentReconcileInterval(DefaultPaymentReconcileIntervalMinutes); err != nil || got != 10 {
		t.Fatalf("mặc định = %d (err %v), muốn 10", got, err)
	}
	for _, ok := range []int{1, 5, 59} {
		if got, err := validatePaymentReconcileInterval(ok); err != nil || got != ok {
			t.Fatalf("%d phải hợp lệ, được %d (err %v)", ok, got, err)
		}
	}
	for _, bad := range []int{0, -1, 60, 1440} {
		if _, err := validatePaymentReconcileInterval(bad); err == nil {
			t.Fatalf("%d phải bị từ chối", bad)
		}
	}
}
