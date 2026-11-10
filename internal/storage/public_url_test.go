package storage

import (
	"testing"

	"study.com/v1/internal/config"
)

// QA-B-02: URL trả cho client phải dùng MINIO_PUBLIC_ENDPOINT, URL client gửi lại phải đổi về được dạng nội bộ.
func TestPublicURL_TachEndpointNoiBoVaCongKhai(t *testing.T) {
	docker := &config.Config{MinioHost: "minio", MinioPort: "9000", MinioPublicEndpoint: "http://localhost:9000/"}
	cdn := &config.Config{MinioHost: "minio", MinioPort: "9000", MinioUseSSL: true, MinioPublicEndpoint: "https://cdn.example.com/minio"}
	unset := &config.Config{MinioHost: "minio", MinioPort: "9000"}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"docker: URL ảnh dùng public endpoint, bỏ dấu / cuối", PublicObjectURL(docker, "images", "general/a.jpg"), "http://localhost:9000/images/general/a.jpg"},
		{"cdn có path + SSL", PublicObjectURL(cdn, "images", "a.jpg"), "https://cdn.example.com/minio/images/a.jpg"},
		{"chưa đặt biến: giữ URL nội bộ như cũ", PublicObjectURL(unset, "images", "a.jpg"), "http://minio:9000/images/a.jpg"},
		{"URL ngoài không bị đổi", PublicizeURL(docker, "https://img.youtube.com/x.jpg"), "https://img.youtube.com/x.jpg"},
		{"không thay nhầm host có cùng tiền tố", PublicizeURL(docker, "http://minio:90001/images/a.jpg"), "http://minio:90001/images/a.jpg"},
		{"presigned giữ query", PublicizeURL(docker, "http://minio:9000/videos/a.mp4?X-Amz-Signature=1"), "http://localhost:9000/videos/a.mp4?X-Amz-Signature=1"},
		{"xoá: URL công khai về dạng nội bộ", InternalizeURL(cdn, "https://cdn.example.com/minio/images/a.jpg"), "https://minio:9000/images/a.jpg"},
		{"xoá: URL nội bộ cũ trong DB giữ nguyên", InternalizeURL(docker, "http://minio:9000/images/a.jpg"), "http://minio:9000/images/a.jpg"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}
