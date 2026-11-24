package config

import (
	"fmt"
	"strings"
)

// validateMinioPublicEndpoint (QA4-T09, gốc Q3-01): khi MINIO_PUBLIC_ENDPOINT rỗng, URL upload/presigned trả cho
// trình duyệt là endpoint NỘI BỘ (MINIO_HOST:MINIO_PORT, vd minio:9000 hay 172.18.0.4:9000), trình duyệt không tới
// được nên upload treo rồi "Failed to fetch". Ngoài dev/test: dừng khởi động thay vì âm thầm trả URL hỏng. Ở dev/test
// vẫn cho chạy (MINIO_HOST=localhost dùng được thẳng) nhưng trả cảnh báo để LoadConfig in ra nếu host không phải loopback.
// Hàm thuần (không đụng viper/flag) để unit test được, cùng lý do validateJWTSecret.
func validateMinioPublicEndpoint(env, publicEndpoint, minioHost string) (warning string, err error) {
	if strings.TrimSpace(publicEndpoint) != "" {
		return "", nil
	}
	if env != "dev" && env != "test" {
		return "", fmt.Errorf("MINIO_PUBLIC_ENDPOINT is required when ENVIRONMENT=%q: without it upload and file URLs point at the internal MinIO address (%s), which browsers cannot reach", env, minioHost)
	}
	switch strings.ToLower(strings.TrimSpace(minioHost)) {
	case "", "localhost", "127.0.0.1", "::1":
		return "", nil
	}
	return fmt.Sprintf("WARNING: MINIO_PUBLIC_ENDPOINT is empty and MINIO_HOST=%s is not a loopback host: browsers will get upload/file URLs they cannot reach. Set MINIO_PUBLIC_ENDPOINT (e.g. http://localhost:9000).", minioHost), nil
}
