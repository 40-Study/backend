package config

import (
	"strings"
	"testing"
)

// QA4-T09: thiếu MINIO_PUBLIC_ENDPOINT ngoài dev/test phải dừng khởi động; ở dev với MINIO_HOST nội bộ phải cảnh báo.
func TestValidateMinioPublicEndpoint(t *testing.T) {
	cases := []struct {
		name, env, endpoint, host string
		wantErr, wantWarn         bool
	}{
		{"prod thiếu endpoint", "prod", "", "minio", true, false},
		{"staging thiếu endpoint (chỉ khoảng trắng)", "staging", "  ", "minio", true, false},
		{"prod có endpoint", "prod", "https://cdn.example.test", "minio", false, false},
		{"dev thiếu endpoint, host nội bộ docker", "dev", "", "172.18.0.4", false, true},
		{"dev thiếu endpoint, localhost", "dev", "", "localhost", false, false},
		{"test thiếu endpoint, host minio", "test", "", "minio", false, true},
		{"dev có endpoint", "dev", "http://localhost:9000", "minio", false, false},
	}
	for _, c := range cases {
		warn, err := validateMinioPublicEndpoint(c.env, c.endpoint, c.host)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v, muốn lỗi=%v", c.name, err, c.wantErr)
		}
		if (warn != "") != c.wantWarn {
			t.Errorf("%s: warning=%q, muốn cảnh báo=%v", c.name, warn, c.wantWarn)
		}
		if err != nil && !strings.Contains(err.Error(), "MINIO_PUBLIC_ENDPOINT") {
			t.Errorf("%s: lỗi phải nêu tên biến cần đặt, được %q", c.name, err)
		}
	}
}
