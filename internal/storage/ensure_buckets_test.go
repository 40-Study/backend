package storage

// Test cho S1 (QA 260929): bucket VIDEO phải private, bucket khác vẫn public-read. Trước đây
// EnsureBuckets đặt public-read cho MỌI bucket nên ai biết đường dẫn object trong bucket videos
// (vd videos/{uploadId}/master.m3u8) đều tải được video qua MinIO/CDN mà không cần chữ ký.
//
// Dùng một S3 giả (httptest) ghi lại policy từng bucket — không cần MinIO thật. Mỗi test ĐỎ khi
// EnsureBuckets lại đặt public-read cho bucket video hoặc không gỡ policy cũ.

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"study.com/v1/internal/config"
)

// fakeS3 mô phỏng đúng các request mà EnsureBuckets gửi: location, HEAD/PUT bucket, PUT/DELETE policy.
type fakeS3 struct {
	mu       sync.Mutex
	buckets  map[string]bool
	policies map[string]string
	// failPolicyDelete: DELETE ?policy trả 403 AccessDenied (không retry) để kiểm lỗi không bị nuốt.
	failPolicyDelete bool
	policyPuts       []string
}

func newFakeS3() *fakeS3 {
	return &fakeS3{buckets: map[string]bool{}, policies: map[string]string{}}
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bucket := strings.Trim(strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)[0], "/")
	q := r.URL.Query()
	_, hasPolicy := q["policy"]
	_, hasLocation := q["location"]

	switch {
	case hasLocation:
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></LocationConstraint>`))
	case hasPolicy && r.Method == http.MethodPut:
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		f.policies[bucket] = string(body)
		f.policyPuts = append(f.policyPuts, bucket)
		w.WriteHeader(http.StatusNoContent)
	case hasPolicy && r.Method == http.MethodDelete:
		if f.failPolicyDelete {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>denied</Message></Error>`))
			return
		}
		delete(f.policies, bucket)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodHead:
		if f.buckets[bucket] {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	case r.Method == http.MethodPut:
		f.buckets[bucket] = true
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func (f *fakeS3) isPublic(bucket string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Contains(f.policies[bucket], "s3:GetObject")
}

func newClientFor(t *testing.T, srv *httptest.Server, cfg config.Config) *MinioClient {
	t.Helper()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MinioHost, cfg.MinioPort = host, port
	cfg.MinioAccessKey, cfg.MinioSecretKey = "test-access", "test-secret-key"
	m, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Server cũ: bucket videos đã tồn tại và đang public-read. Sau EnsureBuckets phải private; ảnh vẫn public.
func TestEnsureBuckets_BucketVideoCuDangPublic_BiGoPolicy_BucketAnhVanPublic(t *testing.T) {
	s3 := newFakeS3()
	s3.buckets["videos"], s3.buckets["images"] = true, true
	s3.policies["videos"] = `{"Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::videos/*"]}]}`
	srv := httptest.NewServer(s3)
	defer srv.Close()

	m := newClientFor(t, srv, config.Config{MinioBucketVideos: "videos", MinioBucketImages: "images", MinIOBucketName: "videos"})
	if err := m.EnsureBuckets(t.Context()); err != nil {
		t.Fatal(err)
	}

	if s3.isPublic("videos") {
		t.Error("bucket videos van public-read sau EnsureBuckets")
	}
	if !s3.isPublic("images") {
		t.Error("bucket images phai van public-read")
	}
}

// Bucket chưa tồn tại: được tạo, video private (không bao giờ có policy public), ảnh public.
func TestEnsureBuckets_TaoMoi_VideoPrivate_AnhPublic(t *testing.T) {
	s3 := newFakeS3()
	srv := httptest.NewServer(s3)
	defer srv.Close()

	m := newClientFor(t, srv, config.Config{MinioBucketVideos: "videos", MinioBucketImages: "images", MinIOBucketName: "videos"})
	if err := m.EnsureBuckets(t.Context()); err != nil {
		t.Fatal(err)
	}

	if !s3.buckets["videos"] || !s3.buckets["images"] {
		t.Fatalf("bucket chua duoc tao: %v", s3.buckets)
	}
	if s3.isPublic("videos") {
		t.Error("bucket videos moi tao khong duoc public")
	}
	for _, b := range s3.policyPuts {
		if b == "videos" {
			t.Error("EnsureBuckets da PUT policy cho bucket videos")
		}
	}
	if !s3.isPublic("images") {
		t.Error("bucket images phai public-read")
	}
}

// MINIO_BUCKET_NAME (bucket chính chứa HLS) khác MINIO_BUCKET_VIDEOS: CẢ HAI đều là bucket video.
func TestEnsureBuckets_BucketChinhKhacBucketVideo_CaHaiPrivate(t *testing.T) {
	s3 := newFakeS3()
	s3.buckets["videos"], s3.buckets["main-videos"], s3.buckets["images"] = true, true, true
	pub := `{"Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::x/*"]}]}`
	s3.policies["videos"], s3.policies["main-videos"] = pub, pub
	srv := httptest.NewServer(s3)
	defer srv.Close()

	m := newClientFor(t, srv, config.Config{MinioBucketVideos: "videos", MinioBucketImages: "images", MinIOBucketName: "main-videos"})
	if err := m.EnsureBuckets(t.Context()); err != nil {
		t.Fatal(err)
	}

	if s3.isPublic("videos") || s3.isPublic("main-videos") {
		t.Errorf("ca hai bucket video phai private: videos=%v main-videos=%v", s3.isPublic("videos"), s3.isPublic("main-videos"))
	}
	if !s3.isPublic("images") {
		t.Error("bucket images phai van public-read")
	}
}

// Không gỡ được policy public của bucket video (vd thiếu quyền) là LỖI, không phải cảnh báo bị nuốt.
func TestEnsureBuckets_KhongGoDuocPolicyBucketVideo_TraLoi(t *testing.T) {
	s3 := newFakeS3()
	s3.buckets["videos"] = true
	s3.failPolicyDelete = true
	srv := httptest.NewServer(s3)
	defer srv.Close()

	m := newClientFor(t, srv, config.Config{MinioBucketVideos: "videos", MinioBucketImages: "images", MinIOBucketName: "videos"})
	if err := m.EnsureBuckets(t.Context()); err == nil {
		t.Fatal("EnsureBuckets phai tra loi khi khong dat duoc bucket video private")
	}
}

// Tên bucket ảnh trùng bucket video: không thể vừa public vừa private -> lỗi rõ, không đặt policy nào.
func TestEnsureBuckets_BucketAnhTrungBucketVideo_TraLoi(t *testing.T) {
	s3 := newFakeS3()
	srv := httptest.NewServer(s3)
	defer srv.Close()

	m := newClientFor(t, srv, config.Config{MinioBucketVideos: "media", MinioBucketImages: "media", MinIOBucketName: "media"})
	if err := m.EnsureBuckets(t.Context()); err == nil {
		t.Fatal("bucket anh trung bucket video phai la loi cau hinh")
	}
	if len(s3.policyPuts) != 0 || s3.isPublic("media") {
		t.Errorf("khong duoc dat policy nao khi cau hinh sai: puts=%v", s3.policyPuts)
	}
}

// Cấu hình thật của dev: MINIO_BUCKET_VIDEOS == MINIO_BUCKET_IMAGES == "study-media", video HLS ở
// MINIO_BUCKET_NAME (mặc định "videos"). Bucket dùng chung phải GIỮ public (ảnh); bucket video private.
func TestEnsureBuckets_BucketVideoCauHinhTrungBucketAnh_AnhVanPublic_HLSPrivate(t *testing.T) {
	s3 := newFakeS3()
	s3.buckets["study-media"], s3.buckets["videos"] = true, true
	pub := `{"Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::x/*"]}]}`
	s3.policies["videos"] = pub
	srv := httptest.NewServer(s3)
	defer srv.Close()

	m := newClientFor(t, srv, config.Config{MinioBucketVideos: "study-media", MinioBucketImages: "study-media", MinIOBucketName: "videos"})
	if err := m.EnsureBuckets(t.Context()); err != nil {
		t.Fatal(err)
	}

	if !s3.isPublic("study-media") {
		t.Error("bucket anh dung chung (study-media) phai giu public-read, neu khong toan bo anh hong")
	}
	if s3.isPublic("videos") {
		t.Error("bucket HLS (MINIO_BUCKET_NAME) phai private")
	}
}
