package service

import (
	"testing"

	"study.com/v1/internal/config"
)

// QA-B-02: POST /api/upload từng trả http://minio:9000/... (endpoint nội bộ Docker) nên ảnh bìa vỡ trên trình duyệt.
// Có MINIO_PUBLIC_ENDPOINT thì URL trả về phải dùng nó; không có thì giữ endpoint nội bộ như cũ.
func TestUploadService_URLTraVeDungPublicEndpoint(t *testing.T) {
	cfg := &config.Config{MinioHost: "minio", MinioPort: "9000", MinioPublicEndpoint: "http://localhost:9000"}
	svc := NewUploadService(nil, cfg)
	if got, want := svc.constructFileURL("images", "general/2026/11/10/a.jpg"), "http://localhost:9000/images/general/2026/11/10/a.jpg"; got != want {
		t.Fatalf("URL upload = %q, muốn %q", got, want)
	}

	cfg.MinioPublicEndpoint = ""
	if got, want := svc.constructFileURL("images", "a.jpg"), "http://minio:9000/images/a.jpg"; got != want {
		t.Fatalf("chưa đặt MINIO_PUBLIC_ENDPOINT: URL = %q, muốn giữ nguyên %q", got, want)
	}
}

// DeleteByURL nhận URL công khai mà client đang giữ; bucket ngoài whitelist vẫn bị chặn trước khi gọi MinIO.
func TestUploadService_DeleteByURL_HieuURLCongKhai(t *testing.T) {
	cfg := &config.Config{MinioHost: "minio", MinioPort: "9000", MinioPublicEndpoint: "https://cdn.example.com/minio",
		MinioBucketImages: "images", MinioBucketVideos: "videos"}
	svc := NewUploadService(nil, cfg)
	// Không đổi về dạng nội bộ thì parts[3] là "minio" và lỗi sẽ là "bucket not allowed: minio".
	err := svc.DeleteByURL(t.Context(), "https://cdn.example.com/minio/secret-bucket/a.jpg")
	if err == nil || err.Error() != "bucket not allowed: secret-bucket" {
		t.Fatalf("err = %v, muốn bucket not allowed: secret-bucket", err)
	}
}
