package config

import (
	"testing"
	"time"
)

// MIGRATE_LOCK_TIMEOUT_MINUTES (L9 mục 3, L10): mặc định 45 (migrate local từng mất ~30 phút), hợp lệ 1-1440, ngoài
// khoảng (kể cả 0 = "không chờ" và âm) thì lỗi; không có cách nào đặt chờ vô hạn.
func TestValidateMigrateLockTimeout(t *testing.T) {
	if got, err := validateMigrateLockTimeout(DefaultMigrateLockTimeoutMinutes); err != nil || got != 45 {
		t.Fatalf("mặc định = %d (err %v), muốn 45", got, err)
	}
	if DefaultMigrateLockTimeoutMinutes != 45 {
		t.Fatalf("DefaultMigrateLockTimeoutMinutes = %d, muốn 45 (hạn chờ khoá phải dài hơn một lần migrate local ~30 phút)", DefaultMigrateLockTimeoutMinutes)
	}
	for _, ok := range []int{1, 30, 1440} {
		if got, err := validateMigrateLockTimeout(ok); err != nil || got != ok {
			t.Fatalf("%d phải hợp lệ, được %d (err %v)", ok, got, err)
		}
	}
	for _, bad := range []int{0, -1, 1441} {
		if _, err := validateMigrateLockTimeout(bad); err == nil {
			t.Fatalf("%d phải bị từ chối", bad)
		}
	}
	if got := (&Config{MigrateLockTimeoutMinutes: 7}).MigrateLockTimeout(); got != 7*time.Minute {
		t.Fatalf("MigrateLockTimeout = %s, muốn 7m", got)
	}
}

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
