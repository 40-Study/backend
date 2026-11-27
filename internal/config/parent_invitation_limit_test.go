package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Q9-PG-01: biến thiếu từng cho hạn mức 0 nên MỌI lời mời phụ huynh bị từ chối ngay lần đầu.
func TestParseParentInvitationDailyLimit(t *testing.T) {
	cases := []struct {
		raw      string
		want     int
		wantWarn bool
	}{
		{"", DefaultParentInvitationDailyLimit, true},
		{"   ", DefaultParentInvitationDailyLimit, true},
		{"abc", DefaultParentInvitationDailyLimit, true},
		{"-3", DefaultParentInvitationDailyLimit, true},
		{"2.5", DefaultParentInvitationDailyLimit, true},
		{"5", 5, false},
		{" 7 ", 7, false},
		{"0", 0, false}, // tường minh 0 = cố ý khoá tính năng
	}
	for _, tc := range cases {
		got, warn := parseParentInvitationDailyLimit(tc.raw)
		if got != tc.want || (warn != "") != tc.wantWarn {
			t.Errorf("parse(%q) = (%d, %q), muốn (%d, cảnh báo=%v)", tc.raw, got, warn, tc.want, tc.wantWarn)
		}
	}
	if DefaultParentInvitationDailyLimit <= 0 {
		t.Fatalf("mặc định phải > 0, got %d", DefaultParentInvitationDailyLimit)
	}
}

// LoadConfig phải nối PARENT_INVITATION_DAILY_LIMIT vào Config: khi biến chỉ có trong môi trường (không có trong .env, như
// qa3_backend) viper chỉ thấy khoá đã SetDefault, và bản cũ để field = 0 -> từ chối mọi lời mời. LoadConfig dùng flag toàn
// cục (gọi 2 lần trong một process sẽ panic "flag redefined"), nên đây là chỗ DUY NHẤT trong package gọi nó.
func TestLoadConfig_ParentInvitationDailyLimit_EnvOnly(t *testing.T) {
	dir := t.TempDir()
	// .env chỉ có JWT_SECRET (bắt buộc); hạn mức lời mời CHỈ nằm trong môi trường.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("JWT_SECRET=unit-test-secret-not-used-elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("PARENT_INVITATION_DAILY_LIMIT", "3")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ParentInvitationDailyLimit != 3 {
		t.Fatalf("PARENT_INVITATION_DAILY_LIMIT=3 chỉ trong môi trường: limit = %d, muốn 3", cfg.ParentInvitationDailyLimit)
	}
}
