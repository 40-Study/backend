package hlsauth

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const testSecret = "unit-test-secret-for-hlsauth-0123456789abcdef"

func setup(t *testing.T) {
	t.Helper()
	if err := Configure(testSecret); err != nil {
		t.Fatal(err)
	}
}

func verifyTok(scope string, id uuid.UUID, tok Token, now time.Time) error {
	uid := ""
	if tok.UID != uuid.Nil {
		uid = tok.UID.String()
	}
	_, err := Verify(scope, id, itoa(tok.Exp), uid, tok.Sig, now)
	return err
}

func itoa(n int64) string {
	q := Token{Exp: n}.Query() // "exp=N&sig="
	return strings.TrimSuffix(strings.TrimPrefix(q, "exp="), "&sig=")
}

func TestSignVerify_DungChuKy_HopLe(t *testing.T) {
	setup(t)
	id, uid := uuid.New(), uuid.New()
	now := time.Now()
	tok, err := Sign(ScopeStream, id, uid, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyTok(ScopeStream, id, tok, now.Add(time.Hour)); err != nil {
		t.Fatalf("chu ky dung, con han phai hop le, loi: %v", err)
	}
}

func TestVerify_HetHan_ErrExpired(t *testing.T) {
	setup(t)
	id := uuid.New()
	now := time.Now()
	tok, _ := Sign(ScopeStream, id, uuid.Nil, now)
	if err := verifyTok(ScopeStream, id, tok, now.Add(TTL+time.Minute)); err != ErrExpired {
		t.Fatalf("het han phai ErrExpired, nhan duoc: %v", err)
	}
}

func TestVerify_ChuKyCuaUploadKhac_ErrInvalid(t *testing.T) {
	setup(t)
	now := time.Now()
	tok, _ := Sign(ScopeStream, uuid.New(), uuid.Nil, now)
	if err := verifyTok(ScopeStream, uuid.New(), tok, now); err != ErrInvalid {
		t.Fatalf("chu ky cua upload khac phai ErrInvalid, nhan duoc: %v", err)
	}
}

// Quyền xem HLS không được mở file gốc: cùng upload, khác scope.
func TestVerify_KhacScope_ErrInvalid(t *testing.T) {
	setup(t)
	id := uuid.New()
	now := time.Now()
	tok, _ := Sign(ScopeStream, id, uuid.Nil, now)
	if err := verifyTok(ScopeOriginal, id, tok, now); err != ErrInvalid {
		t.Fatalf("chu ky scope hls dung cho scope src phai ErrInvalid, nhan duoc: %v", err)
	}
}

func TestVerify_SuaExpHoacUid_ErrInvalid(t *testing.T) {
	setup(t)
	id, uid := uuid.New(), uuid.New()
	now := time.Now()
	tok, _ := Sign(ScopeStream, id, uid, now)

	// Kéo dài hạn: phải hỏng chữ ký.
	if _, err := Verify(ScopeStream, id, itoa(tok.Exp+86400), uid.String(), tok.Sig, now); err != ErrInvalid {
		t.Fatalf("sua exp phai ErrInvalid, nhan duoc: %v", err)
	}
	// Đổi user gắn trong token.
	if _, err := Verify(ScopeStream, id, itoa(tok.Exp), uuid.New().String(), tok.Sig, now); err != ErrInvalid {
		t.Fatalf("sua uid phai ErrInvalid, nhan duoc: %v", err)
	}
	// Bỏ uid (đổi sang khách).
	if _, err := Verify(ScopeStream, id, itoa(tok.Exp), "", tok.Sig, now); err != ErrInvalid {
		t.Fatalf("bo uid phai ErrInvalid, nhan duoc: %v", err)
	}
}

func TestVerify_ThieuHoacRacTham_ErrInvalid(t *testing.T) {
	setup(t)
	id := uuid.New()
	for _, c := range []struct{ exp, uid, sig string }{
		{"", "", ""},
		{"abc", "", "deadbeef"},
		{"9999999999", "khong-phai-uuid", "deadbeef"},
		{"9999999999", "", ""},
	} {
		if _, err := Verify(ScopeStream, id, c.exp, c.uid, c.sig, time.Now()); err != ErrInvalid {
			t.Errorf("Verify(%+v) = %v, muon ErrInvalid", c, err)
		}
	}
}

func TestSign_ChuaCauHinh_Loi(t *testing.T) {
	mu.Lock()
	saved := secret
	secret = nil
	mu.Unlock()
	defer func() { mu.Lock(); secret = saved; mu.Unlock() }()

	if _, err := Sign(ScopeStream, uuid.New(), uuid.Nil, time.Now()); err != ErrNotConfigured {
		t.Fatalf("chua Configure phai ErrNotConfigured (khong duoc tra URL khong ky), nhan duoc: %v", err)
	}
	if _, err := Verify(ScopeStream, uuid.New(), "1", "", "x", time.Now()); err != ErrNotConfigured {
		t.Fatalf("Verify khi chua Configure phai ErrNotConfigured, nhan duoc: %v", err)
	}
}

func TestValidateSecret(t *testing.T) {
	cases := map[string]bool{ // secret -> hợp lệ?
		"":      false,
		"   ":   false,
		"short": false,
		"changeme-generate-with-openssl-rand-hex-32": false,
		"CHANGEME-generate-with-openssl-rand-hex-32": false,
		testSecret: true,
	}
	for s, ok := range cases {
		err := ValidateSecret(s, ".env")
		if (err == nil) != ok {
			t.Errorf("ValidateSecret(%q) err=%v, muon hop le=%v", s, err, ok)
		}
	}
}

func TestExtractUploadID(t *testing.T) {
	id := uuid.New()
	for _, raw := range []string{
		"/api/hls/" + id.String() + "/master.m3u8",
		"/hls/" + id.String() + "/video.mp4",
		"/api/hls/" + id.String() + "/master.m3u8?exp=1&sig=x",
	} {
		got, ok := ExtractUploadID(raw)
		if !ok || got != id {
			t.Errorf("ExtractUploadID(%q) = %v,%v", raw, got, ok)
		}
	}
	for _, raw := range []string{
		"https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4",
		"/api/hls/khong-phai-uuid/master.m3u8",
		"/api/hls/",
		"",
	} {
		if _, ok := ExtractUploadID(raw); ok {
			t.Errorf("ExtractUploadID(%q) khong duoc nhan ra la video noi bo", raw)
		}
	}
}

func TestRewritePlaylist_GanChuKyVaoMoiURITuongDoi(t *testing.T) {
	tok := Token{Exp: 1893456000, UID: uuid.MustParse("11111111-2222-3333-4444-555555555555"), Sig: "abc123"}
	master := "#EXTM3U\r\n#EXT-X-STREAM-INF:BANDWIDTH=800000\r\nv0/index.m3u8\r\n#EXT-X-STREAM-INF:BANDWIDTH=2000000\r\nv1/index.m3u8\r\n"
	got := string(RewritePlaylist([]byte(master), tok))
	q := tok.Query()
	for _, want := range []string{"v0/index.m3u8?" + q, "v1/index.m3u8?" + q, "#EXTM3U", "#EXT-X-STREAM-INF:BANDWIDTH=800000"} {
		if !strings.Contains(got, want) {
			t.Errorf("thieu %q trong:\n%s", want, got)
		}
	}
	// Dòng tag không được bị gắn query.
	if strings.Contains(got, "BANDWIDTH=800000?") {
		t.Errorf("dong tag bi gan query:\n%s", got)
	}

	variant := "#EXTM3U\n#EXTINF:4.0,\nseg_00000.ts\n#EXTINF:4.0,\nseg_00001.ts\n#EXT-X-ENDLIST\n"
	got = string(RewritePlaylist([]byte(variant), tok))
	if strings.Count(got, "?"+q) != 2 || !strings.Contains(got, "seg_00001.ts?"+q) {
		t.Errorf("segment chua duoc gan chu ky:\n%s", got)
	}

	// URI tuyệt đối không phải của ta: giữ nguyên.
	abs := "#EXTM3U\nhttps://cdn.example/x.ts\n"
	if out := string(RewritePlaylist([]byte(abs), tok)); strings.Contains(out, "sig=") {
		t.Errorf("URI tuyet doi khong duoc dong cham:\n%s", out)
	}
}
