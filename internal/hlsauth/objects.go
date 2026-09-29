package hlsauth

import (
	"net/url"
	"path"
	"strings"
	"sync"
)

// Bucket video là PRIVATE (storage.EnsureBuckets gỡ policy public). Ngoài video HLS, bucket này còn
// chứa file phụ đề .vtt do giảng viên upload (URL lưu trong lesson_contents.subtitle_url là URL MinIO
// trực tiếp) và thumbnail. URL MinIO trực tiếp sẽ trả 403, nên phụ đề được phục vụ qua
// GET /api/hls/object bằng URL ký (ScopeObject). Chỉ phụ đề mới được phục vụ: tuyệt đối không mở
// đường đọc video gốc / segment HLS theo khoá object tuỳ ý.

var (
	objMu     sync.RWMutex
	objBucket string
)

// ConfigureObjectBucket đặt tên bucket video (MINIO_BUCKET_NAME) — bucket mà /api/hls/object đọc và
// mà URL phụ đề đã lưu phải thuộc về mới được ký.
func ConfigureObjectBucket(name string) {
	objMu.Lock()
	defer objMu.Unlock()
	objBucket = strings.TrimSpace(name)
}

func currentObjectBucket() string {
	objMu.RLock()
	defer objMu.RUnlock()
	return objBucket
}

// AllowedObjectKey: khoá object được phép phục vụ qua /api/hls/object — chỉ file phụ đề .vtt, không
// có "..", không bắt đầu bằng "/". Dùng ở CẢ hai đầu (lúc ký và lúc phục vụ) để không lệch nhau.
func AllowedObjectKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "/") || !strings.HasSuffix(strings.ToLower(key), ".vtt") {
		return false
	}
	return path.Clean(key) == key && !strings.Contains(key, "..")
}

// ObjectKey trích khoá object từ URL MinIO/CDN đã lưu (dạng .../{bucket}/{key}). Trả false khi URL
// không trỏ vào bucket video hoặc không phải phụ đề hợp lệ — khi đó URL được giữ nguyên (vd URL ngoài).
func ObjectKey(raw string) (string, bool) {
	bucket := currentObjectBucket()
	if bucket == "" {
		return "", false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	rest := strings.TrimPrefix(u.Path, "/")
	prefix := bucket + "/"
	if !strings.HasPrefix(rest, prefix) {
		return "", false
	}
	key := strings.TrimPrefix(rest, prefix)
	if !AllowedObjectKey(key) {
		return "", false
	}
	return key, true
}

// ObjectURL dựng URL tương đối tới /api/hls/object cho một khoá đã ký.
func ObjectURL(key string, t Token) string {
	return "/api/hls/object?key=" + url.QueryEscape(key) + "&" + t.Query()
}
