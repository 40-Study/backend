package handler

// Test cho S1 (QA 260929): /api/hls/* trước đây công khai — ai biết upload_id là tải được toàn bộ
// video, kể cả file gốc. Nay mọi route đòi URL KÝ (hlsauth). Mỗi test dưới đây ĐỎ khi bỏ kiểm chữ
// ký trong HLSHandler.authorize (hoặc bỏ viết lại playlist).

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"study.com/v1/internal/hlsauth"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

const hlsTestSecret = "handler-test-only-hls-secret-0123456789abcdef"

func init() {
	if err := hlsauth.Configure(hlsTestSecret); err != nil {
		panic(err)
	}
}

// fakeHLSStore: kho object trong bộ nhớ, khoá dạng "videos/{id}/...".
type fakeHLSStore struct{ objects map[string][]byte }

func (f *fakeHLSStore) GetDefaultBucket() string { return "test-bucket" }

func (f *fakeHLSStore) StatObject(_ context.Context, _, key string) (minio.ObjectInfo, error) {
	if _, ok := f.objects[key]; !ok {
		return minio.ObjectInfo{}, fiber.ErrNotFound
	}
	return minio.ObjectInfo{Key: key}, nil
}

func (f *fakeHLSStore) GetObject(_ context.Context, _, key string) (io.ReadCloser, error) {
	b, ok := f.objects[key]
	if !ok {
		return nil, fiber.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeHLSStore) ListObjectsWithPrefix(prefix string) ([]minio.ObjectInfo, error) {
	var out []minio.ObjectInfo
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, minio.ObjectInfo{Key: k})
		}
	}
	return out, nil
}

type fakeUploadRepo struct {
	repository.VideoUploadRepositoryInterface
	upload *model.VideoUpload
}

func (f *fakeUploadRepo) GetUploadByID(_ context.Context, id uuid.UUID) (*model.VideoUpload, error) {
	if f.upload != nil && f.upload.ID == id {
		return f.upload, nil
	}
	return nil, nil
}

const masterBody = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000\nv0/index.m3u8\n"
const variantBody = "#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXTINF:4.0,\nseg_00000.ts\n#EXT-X-ENDLIST\n"

// hlsFixture dựng app Fiber với đúng 5 route như router.SetupHLSRoutes.
func hlsFixture(withHLS bool) (*fiber.App, uuid.UUID) {
	id := uuid.New()
	store := &fakeHLSStore{objects: map[string][]byte{"videos/" + id.String() + "/original.mp4": []byte("ORIGINAL")}}
	if withHLS {
		base := "videos/" + id.String() + "/"
		store.objects[base+"master.m3u8"] = []byte(masterBody)
		store.objects[base+"v0/index.m3u8"] = []byte(variantBody)
		store.objects[base+"v0/seg_00000.ts"] = []byte("TSDATA")
	}
	repo := &fakeUploadRepo{upload: &model.VideoUpload{ID: id, Bucket: "test-bucket", ObjectKey: "videos/" + id.String() + "/original.mp4"}}
	h := newHLSHandlerWithStore(store, repo)

	app := fiber.New()
	g := app.Group("/api/hls")
	g.Get("/:upload_id/info", h.GetVideoInfo)
	g.Get("/:upload_id/master.m3u8", h.GetMasterPlaylist)
	g.Get("/:upload_id/video.mp4", h.StreamOriginalVideo)
	g.Get("/:upload_id/:quality/index.m3u8", h.GetPlaylist)
	g.Get("/:upload_id/:quality/:segment", h.GetSegment)
	return app, id
}

func get(t *testing.T, app *fiber.App, target string) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, target, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func signedQuery(t *testing.T, scope string, id uuid.UUID, now time.Time) string {
	t.Helper()
	tok, err := hlsauth.Sign(scope, id, uuid.New(), now)
	if err != nil {
		t.Fatal(err)
	}
	return tok.Query()
}

func allPaths(id uuid.UUID) []string {
	b := "/api/hls/" + id.String()
	return []string{b + "/info", b + "/master.m3u8", b + "/v0/index.m3u8", b + "/v0/seg_00000.ts", b + "/video.mp4"}
}

func TestHLS_KhongCoChuKy_403(t *testing.T) {
	app, id := hlsFixture(true)
	for _, p := range allPaths(id) {
		if code, body := get(t, app, p); code != http.StatusForbidden {
			t.Errorf("GET %s khong chu ky = %d, muon 403 (body: %s)", p, code, body)
		}
	}
}

func TestHLS_ChuKyHetHan_403(t *testing.T) {
	app, id := hlsFixture(true)
	q := signedQuery(t, hlsauth.ScopeStream, id, time.Now().Add(-hlsauth.TTL-time.Hour))
	for _, p := range allPaths(id)[:4] {
		if code, _ := get(t, app, p+"?"+q); code != http.StatusForbidden {
			t.Errorf("GET %s chu ky het han = %d, muon 403", p, code)
		}
	}
	_, body := get(t, app, allPaths(id)[1]+"?"+q)
	if !strings.Contains(body, "HLS_URL_EXPIRED") {
		t.Errorf("het han phai bao code HLS_URL_EXPIRED de web xin URL moi, body: %s", body)
	}
}

func TestHLS_ChuKyCuaUploadKhac_403(t *testing.T) {
	app, id := hlsFixture(true)
	q := signedQuery(t, hlsauth.ScopeStream, uuid.New(), time.Now()) // ký cho upload khác
	for _, p := range allPaths(id)[:4] {
		if code, _ := get(t, app, p+"?"+q); code != http.StatusForbidden {
			t.Errorf("GET %s voi chu ky cua upload khac = %d, muon 403", p, code)
		}
	}
}

func TestHLS_ChuKyDung_200_PlaylistVaSegment(t *testing.T) {
	app, id := hlsFixture(true)
	q := signedQuery(t, hlsauth.ScopeStream, id, time.Now())
	b := "/api/hls/" + id.String()

	code, body := get(t, app, b+"/master.m3u8?"+q)
	if code != http.StatusOK || !strings.Contains(body, "v0/index.m3u8?"+q) {
		t.Fatalf("master = %d, muon 200 va URI con mang chu ky. body: %s", code, body)
	}
	code, body = get(t, app, b+"/v0/index.m3u8?"+q)
	if code != http.StatusOK || !strings.Contains(body, "seg_00000.ts?"+q) {
		t.Fatalf("variant = %d, muon 200 va segment mang chu ky. body: %s", code, body)
	}
	code, body = get(t, app, b+"/v0/seg_00000.ts?"+q)
	if code != http.StatusOK || body != "TSDATA" {
		t.Fatalf("segment = %d %q, muon 200 TSDATA", code, body)
	}
	if code, _ = get(t, app, b+"/info?"+q); code != http.StatusOK {
		t.Fatalf("info = %d, muon 200", code)
	}
}

// Đi đúng như hls.js: chỉ có URL master ký, mọi URL sau lấy từ playlist đã viết lại (tương đối,
// giải theo URL của playlist chứa nó) — segment phải tải được, không cần thêm auth nào.
func TestHLS_ChuoiPlaylistTuMasterToiSegment(t *testing.T) {
	app, id := hlsFixture(true)
	masterURL, _ := url.Parse("/api/hls/" + id.String() + "/master.m3u8?" + signedQuery(t, hlsauth.ScopeStream, id, time.Now()))

	_, master := get(t, app, masterURL.String())
	variantURL := masterURL.ResolveReference(mustRef(t, firstURI(master)))
	code, variant := get(t, app, variantURL.String())
	if code != http.StatusOK {
		t.Fatalf("variant qua URI viet lai = %d, muon 200", code)
	}
	segURL := variantURL.ResolveReference(mustRef(t, firstURI(variant)))
	if code, body := get(t, app, segURL.String()); code != http.StatusOK || body != "TSDATA" {
		t.Fatalf("segment qua chuoi playlist = %d %q, muon 200 TSDATA", code, body)
	}
}

func firstURI(playlist string) string {
	for _, l := range strings.Split(playlist, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			return l
		}
	}
	return ""
}

func mustRef(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// Học viên chỉ có URL scope "hls": KHÔNG được mở file gốc; chỉ URL scope "src" (chủ khoá/admin) mở được.
func TestHLS_FileGoc_ChiChoScopeSrc(t *testing.T) {
	app, id := hlsFixture(true)
	p := "/api/hls/" + id.String() + "/video.mp4"

	if code, _ := get(t, app, p+"?"+signedQuery(t, hlsauth.ScopeStream, id, time.Now())); code != http.StatusForbidden {
		t.Errorf("URL HLS cua hoc vien mo duoc file goc: %d, muon 403", code)
	}
	if code, body := get(t, app, p+"?"+signedQuery(t, hlsauth.ScopeOriginal, id, time.Now())); code != http.StatusOK || body != "ORIGINAL" {
		t.Errorf("URL scope src phai mo duoc file goc, nhan %d %q", code, body)
	}
}

// HLS chưa xử lý xong: trả 202 hls_ready=false, KHÔNG lộ URL video gốc (trước đây có fallback_url).
func TestHLS_ChuaSanSang_KhongLoURLGoc(t *testing.T) {
	app, id := hlsFixture(false)
	q := signedQuery(t, hlsauth.ScopeStream, id, time.Now())
	b := "/api/hls/" + id.String()

	code, body := get(t, app, b+"/master.m3u8?"+q)
	if code != http.StatusAccepted || strings.Contains(body, "video.mp4") || strings.Contains(body, "fallback_url") {
		t.Errorf("master chua san sang = %d, body %s — muon 202 khong lo video goc", code, body)
	}
	code, body = get(t, app, b+"/info?"+q)
	if code != http.StatusOK || strings.Contains(body, "video.mp4") || strings.Contains(body, "fallback_url") || !strings.Contains(body, `"hls_ready":false`) {
		t.Errorf("info chua san sang = %d, body %s — muon hls_ready=false khong lo video goc", code, body)
	}
}

func TestHLS_TranhPathTraversalVaQualityLa(t *testing.T) {
	app, id := hlsFixture(true)
	q := signedQuery(t, hlsauth.ScopeStream, id, time.Now())
	b := "/api/hls/" + id.String()
	if code, _ := get(t, app, b+"/v9/seg_00000.ts?"+q); code != http.StatusBadRequest {
		t.Errorf("quality la phai 400, nhan %d", code)
	}
	if code, _ := get(t, app, b+"/v0/seg_00000.mp4?"+q); code != http.StatusBadRequest {
		t.Errorf("segment khong phai .ts phai 400, nhan %d", code)
	}
}
