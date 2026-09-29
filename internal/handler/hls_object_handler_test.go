package handler

// Test cho S1: phụ đề .vtt trong bucket video (private) chỉ được phục vụ qua /api/hls/object bằng URL
// KÝ scope "obj", và CHỈ file .vtt — chữ ký hợp lệ cũng không mở được video gốc hay segment. Bỏ kiểm
// chữ ký hoặc bỏ AllowedObjectKey trong HLSHandler.GetObject làm các test này ĐỎ.

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/hlsauth"
)

const vttKey = "videos/course/abc/bai1.vtt"

func objectApp() *fiber.App {
	store := &fakeHLSStore{objects: map[string][]byte{
		vttKey:                   []byte("WEBVTT\n\n00:00.000 --> 00:01.000\nxin chao\n"),
		"videos/u1/original.mp4": []byte("ORIGINAL"),
	}}
	h := newHLSHandlerWithStore(store, &fakeUploadRepo{})
	app := fiber.New()
	app.Get("/api/hls/object", h.GetObject)
	return app
}

func objectTarget(t *testing.T, signedKey, requestKey string, now time.Time) string {
	t.Helper()
	tok, err := hlsauth.SignResource(hlsauth.ScopeObject, signedKey, uuid.New(), now)
	if err != nil {
		t.Fatal(err)
	}
	return "/api/hls/object?key=" + url.QueryEscape(requestKey) + "&" + tok.Query()
}

func TestGetObject_KhongChuKy_403(t *testing.T) {
	app := objectApp()
	if code, body := get(t, app, "/api/hls/object?key="+url.QueryEscape(vttKey)); code != http.StatusForbidden {
		t.Errorf("khong chu ky = %d, muon 403 (%s)", code, body)
	}
}

func TestGetObject_ChuKyHopLe_200(t *testing.T) {
	app := objectApp()
	code, body := get(t, app, objectTarget(t, vttKey, vttKey, time.Now()))
	if code != http.StatusOK || len(body) == 0 || body[:6] != "WEBVTT" {
		t.Fatalf("chu ky hop le = %d %q, muon 200 + noi dung phu de", code, body)
	}
}

func TestGetObject_HetHan_403(t *testing.T) {
	app := objectApp()
	past := time.Now().Add(-hlsauth.TTL - time.Hour)
	if code, body := get(t, app, objectTarget(t, vttKey, vttKey, past)); code != http.StatusForbidden {
		t.Errorf("het han = %d, muon 403 (%s)", code, body)
	}
}

// Chữ ký của khoá A không dùng được cho khoá B.
func TestGetObject_ChuKyCuaKhoaKhac_403(t *testing.T) {
	app := objectApp()
	other := "videos/course/other/bai2.vtt"
	if code, body := get(t, app, objectTarget(t, other, vttKey, time.Now())); code != http.StatusForbidden {
		t.Errorf("chu ky khoa khac = %d, muon 403 (%s)", code, body)
	}
}

// Chữ ký scope "hls"/"src" của cùng chuỗi không mở được đường object.
func TestGetObject_ChuKyScopeKhac_403(t *testing.T) {
	app := objectApp()
	tok, err := hlsauth.SignResource(hlsauth.ScopeStream, vttKey, uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if code, body := get(t, app, "/api/hls/object?key="+url.QueryEscape(vttKey)+"&"+tok.Query()); code != http.StatusForbidden {
		t.Errorf("scope khac = %d, muon 403 (%s)", code, body)
	}
}

// Có chữ ký HỢP LỆ cho khoá không phải .vtt vẫn bị từ chối: đường này không bao giờ phục vụ video.
func TestGetObject_KhongPhaiVTT_403DuCoChuKyHopLe(t *testing.T) {
	app := objectApp()
	for _, key := range []string{"videos/u1/original.mp4", "videos/../etc/x.vtt", "/videos/course/abc/bai1.vtt"} {
		if code, body := get(t, app, objectTarget(t, key, key, time.Now())); code != http.StatusForbidden {
			t.Errorf("key %q = %d, muon 403 (%s)", key, code, body)
		}
	}
}
