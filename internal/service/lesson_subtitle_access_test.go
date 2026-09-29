package service

// Test cho S1: bucket video là private, nên URL phụ đề .vtt đã lưu (URL MinIO trực tiếp) không còn
// tải được. API trả URL KÝ của /api/hls/object cho người đã qua kiểm quyền; đường chưa kiểm quyền
// không lộ URL thô; URL ngoài bucket video giữ nguyên. Bỏ signSubtitleURL làm các test này ĐỎ.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/hlsauth"
)

const storedVTT = "http://localhost:9000/videos/videos/course/abc/bai1.vtt"

func withObjectBucket(t *testing.T) {
	t.Helper()
	hlsauth.ConfigureObjectBucket("videos")
	t.Cleanup(func() { hlsauth.ConfigureObjectBucket("") })
}

func TestApplyVideoAccess_PhuDe_KyChoNguoiDaQuaKiemQuyen(t *testing.T) {
	withObjectBucket(t)
	uid := uuid.New()
	stored := storedVTT
	r := dto.LessonContentResponseDTO{SubtitleURL: &stored}
	applyVideoAccess(&r, nil, videoViewer{userID: uid})

	if r.SubtitleURL == nil || !strings.HasPrefix(*r.SubtitleURL, "/api/hls/object?key=") {
		t.Fatalf("phu de phai la URL ky /api/hls/object, nhan: %s", ptrStr(r.SubtitleURL))
	}
	if strings.Contains(*r.SubtitleURL, "localhost:9000") {
		t.Errorf("khong duoc lo URL MinIO tho: %s", *r.SubtitleURL)
	}
	q := hlsauth.QueryOf(*r.SubtitleURL)
	if q.Get("key") != "videos/course/abc/bai1.vtt" {
		t.Errorf("key = %q", q.Get("key"))
	}
	if _, err := hlsauth.VerifyResource(hlsauth.ScopeObject, q.Get("key"), q.Get("exp"), q.Get("uid"), q.Get("sig"), time.Now()); err != nil {
		t.Errorf("URL phu de khong verify duoc: %v", err)
	}
	if q.Get("uid") != uid.String() {
		t.Errorf("chu ky phai gan user_id: uid=%q", q.Get("uid"))
	}
}

func TestApplyVideoAccess_PhuDe_DuongChuaKiemQuyenKhongLoURL(t *testing.T) {
	withObjectBucket(t)
	stored := storedVTT
	r := dto.LessonContentResponseDTO{SubtitleURL: &stored}
	applyVideoAccess(&r, nil, withheldVideoViewer)
	if r.SubtitleURL != nil {
		t.Errorf("duong chua kiem quyen khong duoc tra phu de: %s", *r.SubtitleURL)
	}
}

// Review S1 M5: URL không nằm trong bucket video (bucket khác, URL ngoài, không phải .vtt) KHÔNG được
// trả nguyên dạng thô — bucket video private nên nó chỉ 403 lặng lẽ; bỏ và log cảnh báo.
func TestApplyVideoAccess_PhuDe_URLNgoaiBucketVideo_BoVaKhongTraURLTho(t *testing.T) {
	withObjectBucket(t)
	for _, raw := range []string{
		"https://example.com/sub.vtt",
		"http://localhost:9000/images/a/sub.vtt",
		"http://localhost:9000/videos/course/abc/movie.mp4",
	} {
		s := raw
		r := dto.LessonContentResponseDTO{SubtitleURL: &s}
		applyVideoAccess(&r, nil, guestVideoViewer)
		if r.SubtitleURL != nil {
			t.Errorf("khong ky duoc thi khong duoc tra URL tho: %s -> %s", raw, *r.SubtitleURL)
		}
	}
}
