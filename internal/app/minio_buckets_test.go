package app

// Test cho review S1 F1: bước đặt bucket video private mà lỗi thì backend phải DỪNG khởi động
// (InitResources gọi ensureVideoBucketsPrivate rồi log.Fatalf). Trước đây lỗi chỉ được log warning
// nên backend chạy tiếp với bucket còn public-read. Bỏ việc trả lỗi ở ensureVideoBucketsPrivate làm
// các test này ĐỎ.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"study.com/v1/internal/config"
	"study.com/v1/internal/storage"
)

type stubEnsurer struct{ err error }

func (s stubEnsurer) EnsureBuckets(context.Context) error { return s.err }

func TestEnsureVideoBucketsPrivate_LoiTuEnsureBuckets_LaLoiKhoiDong(t *testing.T) {
	cause := errors.New("boom")
	err := ensureVideoBucketsPrivate(context.Background(), stubEnsurer{err: cause})
	if err == nil {
		t.Fatal("EnsureBuckets loi ma khong tra loi khoi dong")
	}
	if !errors.Is(err, cause) {
		t.Errorf("phai bao ca nguyen nhan goc: %v", err)
	}
	if !strings.Contains(err.Error(), "s3:DeleteBucketPolicy") {
		t.Errorf("thong bao phai neu quyen MinIO can co: %v", err)
	}
}

func TestEnsureVideoBucketsPrivate_ThanhCong_KhongLoi(t *testing.T) {
	if err := ensureVideoBucketsPrivate(context.Background(), stubEnsurer{}); err != nil {
		t.Fatalf("khong loi thi khong duoc tra loi: %v", err)
	}
}

func TestEnsureVideoBucketsPrivate_KhongCoClient_LaLoi(t *testing.T) {
	if err := ensureVideoBucketsPrivate(context.Background(), nil); err == nil {
		t.Fatal("khong co client MinIO thi khong dat duoc bucket private, phai la loi")
	}
}

// Cấu hình sai: MINIO_BUCKET_NAME trùng MINIO_BUCKET_IMAGES. Dùng MinioClient THẬT (lỗi cấu hình
// trả về trước mọi request mạng nên trỏ vào cổng không có ai lắng nghe).
func TestEnsureVideoBucketsPrivate_BucketVideoTrungBucketAnh_LaLoiKhoiDong(t *testing.T) {
	m, err := storage.NewMinioClient(&config.Config{
		MinioHost: "127.0.0.1", MinioPort: "1", MinioAccessKey: "a", MinioSecretKey: "b",
		MinioBucketImages: "study-media", MinioBucketVideos: "study-media", MinIOBucketName: "study-media",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = ensureVideoBucketsPrivate(context.Background(), m)
	if err == nil {
		t.Fatal("bucket video trung bucket anh phai lam khoi dong that bai")
	}
	if !strings.Contains(err.Error(), "MINIO_BUCKET_NAME") {
		t.Errorf("thong bao phai chi ra MINIO_BUCKET_NAME: %v", err)
	}
}

// Gỡ policy public-read thất bại (credential thiếu quyền DeleteBucketPolicy): MinIO giả trả 403
// AccessDenied cho DELETE ?policy.
func TestEnsureVideoBucketsPrivate_GoPolicyThatBai_LaLoiKhoiDong(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		_, hasPolicy := q["policy"]
		_, hasLocation := q["location"]
		switch {
		case hasLocation:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></LocationConstraint>`))
		case hasPolicy && r.Method == http.MethodDelete:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>denied</Message></Error>`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	host, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := storage.NewMinioClient(&config.Config{
		MinioHost: host, MinioPort: port, MinioAccessKey: "a", MinioSecretKey: "b",
		MinioBucketImages: "images", MinioBucketVideos: "videos", MinIOBucketName: "videos",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureVideoBucketsPrivate(context.Background(), m); err == nil {
		t.Fatal("go policy that bai phai lam khoi dong that bai")
	}
}
