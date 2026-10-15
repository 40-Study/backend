package config

import (
	"testing"
	"time"
)

// JUDGE0_URL (contract sec. 8): rỗng thì giữ đúng hành vi cũ (localhost khi dev, host docker "judge0" khi khác);
// có giá trị thì dùng nguyên (bỏ "/" cuối). LoadConfig dùng flag/viper toàn cục nên chỉ test hàm thuần.
func TestResolveJudge0URL(t *testing.T) {
	cases := []struct{ raw, env, want string }{
		{"", "dev", "http://localhost:2358"},
		{"", "prod", "http://judge0:2358"},
		{"", "test", "http://judge0:2358"},
		{"  ", "dev", "http://localhost:2358"},
		{"http://j0.internal:9000", "dev", "http://j0.internal:9000"},
		{"https://judge.example.com/", "prod", "https://judge.example.com"},
	}
	for _, tc := range cases {
		if got := ResolveJudge0URL(tc.raw, tc.env); got != tc.want {
			t.Errorf("ResolveJudge0URL(%q, %q) = %q, muốn %q", tc.raw, tc.env, got, tc.want)
		}
	}
}

func TestJudge0Timeout(t *testing.T) {
	if got := (&Config{}).Judge0Timeout(); got != DefaultJudge0TimeoutSeconds*time.Second {
		t.Errorf("Config rỗng (dựng tay trong test) = %v, muốn mặc định %ds", got, DefaultJudge0TimeoutSeconds)
	}
	if got := (&Config{Judge0TimeoutSeconds: 12}).Judge0Timeout(); got != 12*time.Second {
		t.Errorf("12 -> %v", got)
	}
	if v, err := validateJudge0Timeout(DefaultJudge0TimeoutSeconds); err != nil || v != 30 {
		t.Errorf("mặc định 30 = %d, %v", v, err)
	}
	for _, bad := range []int{0, -1, 301} {
		if _, err := validateJudge0Timeout(bad); err == nil {
			t.Errorf("JUDGE0_TIMEOUT_SECONDS=%d phải bị từ chối lúc khởi động", bad)
		}
	}
}
