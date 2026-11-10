package storage

import (
	"fmt"
	"strings"

	"study.com/v1/internal/config"
)

// Hai địa chỉ của cùng một MinIO:
//   - nội bộ: MINIO_HOST:MINIO_PORT, backend dùng để gọi SDK (trong Docker thường là "minio:9000");
//   - công khai: MINIO_PUBLIC_ENDPOINT, trình duyệt dùng để mở file (VD: http://localhost:9000, https://cdn...).
// QA-B-02 (261110): POST /api/upload từng trả URL nội bộ http://minio:9000/... nên ảnh bìa vỡ trên trình duyệt.
// Mọi URL đi ra client phải qua PublicizeURL; mọi URL client gửi ngược lại (xoá file) đi qua InternalizeURL.

// internalBaseURL: "http(s)://MINIO_HOST:MINIO_PORT", không có dấu "/" cuối.
func internalBaseURL(cfg *config.Config) string {
	protocol := "http"
	if cfg.MinioUseSSL {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s:%s", protocol, cfg.MinioHost, cfg.MinioPort)
}

// publicBaseURL: MINIO_PUBLIC_ENDPOINT bỏ dấu "/" cuối; rỗng = chưa cấu hình.
func publicBaseURL(cfg *config.Config) string {
	return strings.TrimRight(strings.TrimSpace(cfg.MinioPublicEndpoint), "/")
}

// InternalObjectURL: URL của object theo endpoint nội bộ (định dạng lưu trước nay: base/bucket/object).
func InternalObjectURL(cfg *config.Config, bucket, objectKey string) string {
	return internalBaseURL(cfg) + "/" + bucket + "/" + objectKey
}

// PublicizeURL thay tiền tố endpoint nội bộ bằng MINIO_PUBLIC_ENDPOINT. Không cấu hình public endpoint, hoặc URL
// không bắt đầu bằng endpoint nội bộ (URL ngoài, đã là public) thì trả nguyên — giữ hành vi cũ khi chưa đặt biến.
func PublicizeURL(cfg *config.Config, rawURL string) string {
	public := publicBaseURL(cfg)
	if public == "" {
		return rawURL
	}
	internal := internalBaseURL(cfg)
	if !strings.HasPrefix(rawURL, internal+"/") && rawURL != internal {
		return rawURL
	}
	return public + strings.TrimPrefix(rawURL, internal)
}

// InternalizeURL là chiều ngược của PublicizeURL: URL công khai (client gửi lên để xoá file) đổi về dạng nội bộ để
// tách bucket/object theo vị trí như trước. Cần vì MINIO_PUBLIC_ENDPOINT có thể có path (https://cdn/x/minio), khi
// đó vị trí bucket trong URL công khai lệch so với URL nội bộ.
func InternalizeURL(cfg *config.Config, rawURL string) string {
	public := publicBaseURL(cfg)
	if public == "" || !strings.HasPrefix(rawURL, public+"/") {
		return rawURL
	}
	return internalBaseURL(cfg) + strings.TrimPrefix(rawURL, public)
}

// PublicObjectURL: URL công khai của object, dùng cho mọi URL trả về client.
func PublicObjectURL(cfg *config.Config, bucket, objectKey string) string {
	return PublicizeURL(cfg, InternalObjectURL(cfg, bucket, objectKey))
}
