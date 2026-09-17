package service

// Test cho V-I (review web): POST /api/videos/upload/complete phải trả về `url` của chính object
// vừa upload. Trước đây response chỉ có object_key nên web không dựng được URL của file nó vừa
// upload — cả luồng upload phụ đề .vtt không dùng được vì không màn hình nào hiển thị URL đó.
//
// Test đi qua newCompleteResponse (không qua CompleteVideoUpload) vì hàm đó gọi MinIO thật
// (CompleteMultipartUpload) mà VideoUploadService giữ *storage.MinioClient concrete.

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/storage"
)

// newServiceCoMinioChoTestURL dựng VideoUploadService chỉ với storage client — đủ cho
// newCompleteResponse. storage.NewMinioClient không kết nối mạng, chỉ build SDK client từ config.
func newServiceCoMinioChoTestURL(t *testing.T, cfg *config.Config) *VideoUploadService {
	t.Helper()
	client, err := storage.NewMinioClient(cfg)
	if err != nil {
		t.Fatalf("khong tao duoc MinioClient: %v", err)
	}
	return &VideoUploadService{storage: client}
}

// TestNewCompleteResponse_CoURLDungBucketVaObjectKey khoá lại yêu cầu chính: `url` phải được
// populate và phải khớp chính xác bucket + object_key của upload record (đây cũng là cặp giá trị
// client đã upload part lên — xem GetPresignedURLs — nên URL không trỏ sai object).
func TestNewCompleteResponse_CoURLDungBucketVaObjectKey(t *testing.T) {
	cfg := &config.Config{
		MinioHost:   "localhost",
		MinioPort:   "9000",
		MinioUseSSL: false,
	}
	svc := newServiceCoMinioChoTestURL(t, cfg)

	upload := &model.VideoUpload{
		ID:        uuid.New(),
		Bucket:    "videos",
		ObjectKey: "videos/course/9f1c0e2a/bai1.vtt",
	}

	resp := svc.newCompleteResponse(upload, true, model.VideoUploadStatusProcessing, "ok")

	want := "http://localhost:9000/videos/videos/course/9f1c0e2a/bai1.vtt"
	if resp.URL != want {
		t.Errorf("URL = %q, muon %q", resp.URL, want)
	}
	if resp.ObjectKey != upload.ObjectKey {
		t.Errorf("ObjectKey = %q, muon %q", resp.ObjectKey, upload.ObjectKey)
	}
	if resp.UploadID != upload.ID {
		t.Errorf("UploadID = %s, muon %s", resp.UploadID, upload.ID)
	}
}

// TestNewCompleteResponse_JSONKeyLaURL khoá lại tên field: web đọc `data.url`, đổi tên key là
// phá client im lặng (field chỉ đơn giản biến mất khỏi JSON).
func TestNewCompleteResponse_JSONKeyLaURL(t *testing.T) {
	cfg := &config.Config{MinioHost: "localhost", MinioPort: "9000"}
	svc := newServiceCoMinioChoTestURL(t, cfg)

	resp := svc.newCompleteResponse(&model.VideoUpload{
		ID:        uuid.New(),
		Bucket:    "videos",
		ObjectKey: "videos/course/abc/bai1.vtt",
	}, true, model.VideoUploadStatusProcessing, "ok")

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response loi: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal response loi: %v", err)
	}

	got, ok := payload["url"]
	if !ok {
		t.Fatalf("JSON khong co key \"url\": %s", string(raw))
	}
	if got != resp.URL {
		t.Errorf("payload[\"url\"] = %v, muon %q", got, resp.URL)
	}
}

// TestNewCompleteResponse_DungMinioPublicEndpoint: URL này đi thẳng ra browser, nên khi
// MINIO_PUBLIC_ENDPOINT được cấu hình (production) nó phải dùng endpoint public thay vì
// host:port nội bộ (VD: minio:9000 — browser không resolve được).
func TestNewCompleteResponse_DungMinioPublicEndpoint(t *testing.T) {
	cfg := &config.Config{
		MinioHost:           "minio",
		MinioPort:           "9000",
		MinioPublicEndpoint: "https://cdn.example.com",
	}
	svc := newServiceCoMinioChoTestURL(t, cfg)

	resp := svc.newCompleteResponse(&model.VideoUpload{
		ID:        uuid.New(),
		Bucket:    "videos",
		ObjectKey: "videos/course/abc/bai1.vtt",
	}, true, model.VideoUploadStatusProcessing, "ok")

	want := "https://cdn.example.com/videos/videos/course/abc/bai1.vtt"
	if resp.URL != want {
		t.Errorf("URL = %q, muon %q", resp.URL, want)
	}
}

// TestCompleteVideoUploadResponse_CoFieldURL chiếm chỗ compile-time: nếu ai đó xoá field URL
// khỏi DTO thì file test này không build được nữa.
func TestCompleteVideoUploadResponse_CoFieldURL(t *testing.T) {
	var resp dto.CompleteVideoUploadResponse
	resp.URL = "http://localhost:9000/videos/videos/a.vtt"
	if resp.URL == "" {
		t.Fatal("field URL khong ton tai tren CompleteVideoUploadResponse")
	}
}
