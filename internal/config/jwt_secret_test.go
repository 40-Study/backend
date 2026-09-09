package config

import "testing"

// TestValidateJWTSecret (H-04, audit 260909 vòng 2) — trước đây LoadConfig có
// viper.SetDefault("JWT_SECRET", "supersecretkey-change-in-production"); nếu deploy thiếu
// biến môi trường JWT_SECRET, app vẫn khởi động và ký JWT bằng secret công khai nằm thẳng
// trong source code. validateJWTSecret phải fail-fast (trả lỗi) khi secret rỗng hoặc vẫn còn
// giá trị mặc định cũ, và phải cho qua khi secret là một chuỗi thật bất kỳ khác.
func TestValidateJWTSecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"rỗng bị từ chối", "", true},
		{"chỉ khoảng trắng bị từ chối", "   ", true},
		{"giá trị mặc định cũ bị từ chối", "supersecretkey-change-in-production", true},
		{"secret thật hợp lệ", "a-long-random-secret-generated-by-ops-team", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWTSecret(tt.secret, ".env")
			if tt.wantErr && err == nil {
				t.Errorf("validateJWTSecret(%q) = nil, want error", tt.secret)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateJWTSecret(%q) = %v, want nil", tt.secret, err)
			}
		})
	}
}
