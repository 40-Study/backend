package hlsauth

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Test cho S1: phụ đề .vtt nằm trong bucket video (private) được phục vụ qua URL ký scope "obj".

func TestObjectKey_ChiNhanPhuDeTrongBucketVideo(t *testing.T) {
	ConfigureObjectBucket("videos")
	defer ConfigureObjectBucket("")

	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"http://localhost:9000/videos/videos/course/abc/bai1.vtt", "videos/course/abc/bai1.vtt", true},
		{"https://cdn.fortex.ai.vn/videos/videos/course/abc/BAI1.VTT", "videos/course/abc/BAI1.VTT", true},
		// Không phải phụ đề: video gốc / segment / playlist không bao giờ được ký theo khoá object.
		{"http://localhost:9000/videos/videos/course/abc/bai1.mp4", "", false},
		{"http://localhost:9000/videos/videos/u/master.m3u8", "", false},
		// Bucket khác (ảnh) hoặc URL ngoài: giữ nguyên, không ký.
		{"http://localhost:9000/images/a/b.vtt", "", false},
		{"https://example.com/sub.vtt", "", false},
		// Thoát thư mục.
		{"http://localhost:9000/videos/../secret/x.vtt", "", false},
		{"http://localhost:9000/videos/a/../../x.vtt", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := ObjectKey(c.raw)
		if ok != c.ok || got != c.want {
			t.Errorf("ObjectKey(%q) = (%q,%v), muon (%q,%v)", c.raw, got, ok, c.want, c.ok)
		}
	}
}

func TestObjectKey_ChuaCauHinhBucket_KhongKyGi(t *testing.T) {
	ConfigureObjectBucket("")
	if _, ok := ObjectKey("http://localhost:9000/videos/a.vtt"); ok {
		t.Error("chua cau hinh bucket thi khong duoc ky")
	}
}

func TestSignResource_ScopeVaKhoaTachBiet(t *testing.T) {
	if err := Configure("hlsauth-objects-test-secret-0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	uid := uuid.New()
	tok, err := SignResource(ScopeObject, "videos/course/a/x.vtt", uid, now)
	if err != nil {
		t.Fatal(err)
	}
	verify := func(scope, res string, at time.Time) error {
		_, err := VerifyResource(scope, res, strconv.FormatInt(tok.Exp, 10), uid.String(), tok.Sig, at)
		return err
	}
	if err := verify(ScopeObject, "videos/course/a/x.vtt", now); err != nil {
		t.Fatalf("chu ky hop le bi tu choi: %v", err)
	}
	if err := verify(ScopeObject, "videos/course/a/y.vtt", now); err != ErrInvalid {
		t.Errorf("chu ky cua khoa khac phai ErrInvalid, nhan %v", err)
	}
	if err := verify(ScopeStream, "videos/course/a/x.vtt", now); err != ErrInvalid {
		t.Errorf("chu ky scope obj khong duoc dung o scope hls, nhan %v", err)
	}
	if err := verify(ScopeObject, "videos/course/a/x.vtt", now.Add(TTL+time.Minute)); err != ErrExpired {
		t.Errorf("het han phai ErrExpired, nhan %v", err)
	}
}

func TestObjectURL_MaHoaKhoaVaMangChuKy(t *testing.T) {
	u := ObjectURL("videos/a b/x.vtt", Token{Exp: 5, Sig: "abc"})
	if !strings.HasPrefix(u, "/api/hls/object?key=videos%2Fa+b%2Fx.vtt&exp=5") || !strings.HasSuffix(u, "&sig=abc") {
		t.Errorf("url = %s", u)
	}
}
