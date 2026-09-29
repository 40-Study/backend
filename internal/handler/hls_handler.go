package handler

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"study.com/v1/internal/hlsauth"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/storage"
)

// Rate limit: 10 MB/s for video streaming
const streamRateLimitBytesPerSec = 10 * 1024 * 1024

// maxPlaylistBytes chặn đọc playlist quá lớn vào bộ nhớ — playlist HLS thật chỉ vài KB.
const maxPlaylistBytes = 1 << 20

// Toàn bộ /api/hls/* đòi URL KÝ (hlsauth). Handler KHÔNG bao giờ redirect sang URL presigned của
// MinIO cho playlist: playlist con/segment trong đó là URI tương đối sẽ trỏ thẳng MinIO, thoát
// khỏi kiểm chữ ký. Playlist luôn đi qua backend và được viết lại để mang chữ ký xuống từng URI.
type HLSHandler struct {
	minioClient hlsObjectStore
	uploadRepo  repository.VideoUploadRepositoryInterface
}

// hlsObjectStore là phần của storage.MinioClient mà handler HLS dùng — tách interface để test
// được đường 200 (playlist viết lại, segment) mà không cần MinIO thật.
type hlsObjectStore interface {
	GetDefaultBucket() string
	StatObject(ctx context.Context, bucket, objectKey string) (minio.ObjectInfo, error)
	GetObject(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error)
	ListObjectsWithPrefix(prefix string) ([]minio.ObjectInfo, error)
}

func buildHLSKeyCandidates(uploadID string, parts ...string) []string {
	baseWithPrefix := append([]string{"videos", uploadID}, parts...)
	baseWithoutPrefix := append([]string{uploadID}, parts...)

	return []string{
		path.Join(baseWithPrefix...),
		path.Join(baseWithoutPrefix...),
	}
}

func (h *HLSHandler) resolveExistingHLSObjectKey(ctx *fiber.Ctx, candidates []string) (string, error) {
	bucket := h.minioClient.GetDefaultBucket()
	var lastErr error
	for _, key := range candidates {
		if _, err := h.minioClient.StatObject(ctx.Context(), bucket, key); err == nil {
			return key, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fiber.ErrNotFound
	}
	return "", lastErr
}

func NewHLSHandler(minioClient *storage.MinioClient, uploadRepo repository.VideoUploadRepositoryInterface) *HLSHandler {
	return &HLSHandler{minioClient: minioClient, uploadRepo: uploadRepo}
}

// newHLSHandlerWithStore dùng cho test: thay kho MinIO bằng bản giả.
func newHLSHandlerWithStore(store hlsObjectStore, uploadRepo repository.VideoUploadRepositoryInterface) *HLSHandler {
	return &HLSHandler{minioClient: store, uploadRepo: uploadRepo}
}

// authorize kiểm upload_id trên path + chữ ký trên query theo scope. Trả ok=true khi hợp lệ;
// ngược lại response lỗi (400/403/500) ĐÃ được ghi và handler chỉ việc `return nil`.
// (Không trả response dưới dạng error: c.JSON(...) trả nil khi ghi thành công nên sẽ thành "cho qua".)
//
// 403 (không phải 401): người gọi không có danh tính để xác thực, chỉ thiếu/sai/hết hạn quyền
// truy cập URL này — web dựa vào 403 để xin URL ký mới đúng một lần.
func (h *HLSHandler) authorize(c *fiber.Ctx, scope string) (uploadID uuid.UUID, tok hlsauth.Token, ok bool) {
	uploadID, err := uuid.Parse(c.Params("upload_id"))
	if err != nil {
		_ = c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id không hợp lệ"})
		return uuid.Nil, hlsauth.Token{}, false
	}
	tok, err = hlsauth.Verify(scope, uploadID, c.Query("exp"), c.Query("uid"), c.Query("sig"), time.Now())
	switch err {
	case nil:
		return uploadID, tok, true
	case hlsauth.ErrExpired:
		_ = c.Status(http.StatusForbidden).JSON(fiber.Map{
			"error": "Liên kết video đã hết hạn. Vui lòng tải lại trang để tiếp tục xem.",
			"code":  "HLS_URL_EXPIRED",
		})
	case hlsauth.ErrInvalid:
		_ = c.Status(http.StatusForbidden).JSON(fiber.Map{
			"error": "Bạn không có quyền xem video này.",
			"code":  "HLS_URL_INVALID",
		})
	default:
		// Chưa cấu hình secret: InitResources đã chặn khởi động nên đây là lỗi hiếm; báo lỗi rõ.
		_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Dịch vụ video chưa được cấu hình"})
	}
	return uuid.Nil, hlsauth.Token{}, false
}

// maxSubtitleBytes chặn đọc file phụ đề quá lớn vào bộ nhớ — file .vtt thật chỉ vài chục KB.
const maxSubtitleBytes = 5 << 20

// GetObject phục vụ file phụ đề .vtt nằm trong bucket video (private) qua URL ký scope "obj".
// Chữ ký phủ cả khoá object, và khoá phải qua hlsauth.AllowedObjectKey (chỉ .vtt) nên URL này không
// bao giờ đọc được video gốc / segment HLS dù có chữ ký hợp lệ của tài nguyên khác.
// Route: GET /hls/object?key=...&exp=..&uid=..&sig=..
func (h *HLSHandler) GetObject(c *fiber.Ctx) error {
	key := c.Query("key")
	if !hlsauth.AllowedObjectKey(key) {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{
			"error": "Bạn không có quyền xem tệp này.",
			"code":  "HLS_URL_INVALID",
		})
	}
	if _, err := hlsauth.VerifyResource(hlsauth.ScopeObject, key, c.Query("exp"), c.Query("uid"), c.Query("sig"), time.Now()); err != nil {
		switch err {
		case hlsauth.ErrExpired:
			return c.Status(http.StatusForbidden).JSON(fiber.Map{
				"error": "Liên kết đã hết hạn. Vui lòng tải lại trang.",
				"code":  "HLS_URL_EXPIRED",
			})
		case hlsauth.ErrInvalid:
			return c.Status(http.StatusForbidden).JSON(fiber.Map{
				"error": "Bạn không có quyền xem tệp này.",
				"code":  "HLS_URL_INVALID",
			})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Dịch vụ video chưa được cấu hình"})
		}
	}

	bucket := h.minioClient.GetDefaultBucket()
	info, err := h.minioClient.StatObject(c.Context(), bucket, key)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Không tìm thấy tệp"})
	}
	if info.Size > maxSubtitleBytes {
		return c.Status(http.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": "Tệp quá lớn"})
	}
	reader, err := h.minioClient.GetObject(c.Context(), bucket, key)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Không đọc được tệp"})
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, maxSubtitleBytes+1))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Không đọc được tệp"})
	}
	c.Set("Content-Type", "text/vtt; charset=utf-8")
	c.Set("Cache-Control", "private, max-age=3600")
	return c.Status(http.StatusOK).Send(body)
}

// StreamOriginalVideo streams the original video file. CHỈ phục vụ URL scope "src" — loại URL
// mà API chỉ cấp cho chủ khoá học / admin; URL HLS của học viên không mở được file gốc.
// Route: GET /hls/:upload_id/video.mp4
func (h *HLSHandler) StreamOriginalVideo(c *fiber.Ctx) error {
	uploadID, _, ok := h.authorize(c, hlsauth.ScopeOriginal)
	if !ok {
		return nil
	}

	upload, err := h.uploadRepo.GetUploadByID(c.Context(), uploadID)
	if err != nil || upload == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Video not found"})
	}

	// Get video from MinIO
	reader, err := h.minioClient.GetObject(c.Context(), upload.Bucket, upload.ObjectKey)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get video"})
	}
	defer reader.Close()

	// Set headers for video streaming
	c.Set("Content-Type", "video/mp4")
	c.Set("Accept-Ranges", "bytes")
	// private: đây là nội dung có kiểm quyền, không để proxy/CDN dùng chung lưu.
	c.Set("Cache-Control", "private, max-age=3600")

	// Stream with rate limiting
	return streamWithRateLimit(c, reader, streamRateLimitBytesPerSec)
}

// servePlaylist đọc playlist từ MinIO, gắn chữ ký vào mọi URI con rồi trả về.
func (h *HLSHandler) servePlaylist(c *fiber.Ctx, tok hlsauth.Token, objectKey string) error {
	bucket := h.minioClient.GetDefaultBucket()
	reader, err := h.minioClient.GetObject(c.Context(), bucket, objectKey)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get playlist"})
	}
	defer reader.Close()

	body, err := io.ReadAll(io.LimitReader(reader, maxPlaylistBytes))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to read playlist"})
	}

	c.Set("Content-Type", "application/vnd.apple.mpegurl")
	// no-store: playlist mang chữ ký hết hạn, không được cache dùng lại sau khi hết hạn.
	c.Set("Cache-Control", "private, no-store")
	return c.Send(hlsauth.RewritePlaylist(body, tok))
}

// GetMasterPlaylist phục vụ master.m3u8
// Route: GET /hls/:upload_id/master.m3u8
// Object key: videos/{upload_id}/master.m3u8
// HLS chưa sẵn sàng -> 202 kèm hls_ready=false. KHÔNG trả URL video gốc: học viên/khách không
// được xem file gốc (chủ khoá/admin nhận URL gốc riêng từ API nội dung bài học).
func (h *HLSHandler) GetMasterPlaylist(c *fiber.Ctx) error {
	uploadID, tok, ok := h.authorize(c, hlsauth.ScopeStream)
	if !ok {
		return nil
	}

	objectKey, err := h.resolveExistingHLSObjectKey(c, buildHLSKeyCandidates(uploadID.String(), "master.m3u8"))
	if err != nil {
		if up, upErr := h.uploadRepo.GetUploadByID(c.Context(), uploadID); upErr != nil || up == nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Video not found"})
		}
		return c.Status(http.StatusAccepted).JSON(fiber.Map{
			"hls_ready": false,
			"message":   "Video đang được xử lý, vui lòng quay lại sau ít phút",
		})
	}
	return h.servePlaylist(c, tok, objectKey)
}

// GetPlaylist phục vụ playlist của từng quality
// Route: GET /hls/:upload_id/:quality/index.m3u8
// :quality là "v0" (480p), "v1" (720p), "v2" (1080p)
// Object key: videos/{upload_id}/{quality}/index.m3u8
func (h *HLSHandler) GetPlaylist(c *fiber.Ctx) error {
	uploadID, tok, ok := h.authorize(c, hlsauth.ScopeStream)
	if !ok {
		return nil
	}
	quality := c.Params("quality") // "v0", "v1", "v2"

	if quality == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id and quality are required"})
	}

	// Chỉ cho phép v0, v1, v2
	if quality != "v0" && quality != "v1" && quality != "v2" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "quality must be v0, v1, or v2"})
	}

	objectKey, err := h.resolveExistingHLSObjectKey(c, buildHLSKeyCandidates(uploadID.String(), quality, "index.m3u8"))
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Playlist not found. Video may still be processing.",
		})
	}
	return h.servePlaylist(c, tok, objectKey)
}

// GetSegment phục vụ từng .ts segment với rate limiting
// Route: GET /hls/:upload_id/:quality/:segment
// :quality là "v0"/"v1"/"v2", :segment là "seg_00001.ts"
// Object key: videos/{upload_id}/{quality}/{segment}
func (h *HLSHandler) GetSegment(c *fiber.Ctx) error {
	uploadID, _, ok := h.authorize(c, hlsauth.ScopeStream)
	if !ok {
		return nil
	}
	quality := c.Params("quality")
	segment := c.Params("segment")

	if quality == "" || segment == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id, quality and segment are required"})
	}

	// Validate — chống path traversal
	if strings.Contains(quality, "/") || strings.Contains(quality, "..") ||
		strings.Contains(segment, "/") || strings.Contains(segment, "..") {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid parameters"})
	}

	if quality != "v0" && quality != "v1" && quality != "v2" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "quality must be v0, v1, or v2"})
	}

	if !strings.HasSuffix(segment, ".ts") {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Only .ts segments are allowed"})
	}

	bucket := h.minioClient.GetDefaultBucket()
	objectKey, err := h.resolveExistingHLSObjectKey(c, buildHLSKeyCandidates(uploadID.String(), quality, segment))
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Segment not found. Video may still be processing.",
		})
	}

	// Stream with rate limiting instead of redirect
	reader, err := h.minioClient.GetObject(c.Context(), bucket, objectKey)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get segment"})
	}
	defer reader.Close()

	c.Set("Content-Type", "video/mp2t")
	// private: segment có kiểm quyền — không cho cache dùng chung (CDN/proxy) lưu 1 năm như trước.
	c.Set("Cache-Control", "private, max-age=3600")

	// Rate-limited streaming
	return streamWithRateLimit(c, reader, streamRateLimitBytesPerSec)
}

// streamWithRateLimit streams data with bandwidth throttling
func streamWithRateLimit(c *fiber.Ctx, reader io.Reader, bytesPerSec int) error {
	buf := make([]byte, 64*1024) // 64KB chunks
	bytesSent := 0
	startTime := time.Now()

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			// Write chunk
			if _, writeErr := c.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			bytesSent += n

			// Throttle: calculate how long we should have taken
			elapsed := time.Since(startTime).Seconds()
			expectedTime := float64(bytesSent) / float64(bytesPerSec)
			if sleepTime := expectedTime - elapsed; sleepTime > 0 {
				time.Sleep(time.Duration(sleepTime * float64(time.Second)))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// GetVideoInfo trả về trạng thái HLS của video (đã sẵn sàng hay đang xử lý).
// Route: GET /hls/:upload_id/info — đòi URL ký scope "hls" như các route còn lại.
// KHÔNG trả URL video gốc (fallback_url đã bị bỏ): trước đây field này lộ file gốc cho bất kỳ ai.
func (h *HLSHandler) GetVideoInfo(c *fiber.Ctx) error {
	uploadID, tok, ok := h.authorize(c, hlsauth.ScopeStream)
	if !ok {
		return nil
	}
	id := uploadID.String()

	prefixes := []string{
		path.Join("videos", id) + "/",
		id + "/",
	}

	allObjects := make([]struct{ Key string }, 0)
	seen := map[string]struct{}{}
	for _, prefix := range prefixes {
		objects, err := h.minioClient.ListObjectsWithPrefix(prefix)
		if err != nil {
			continue
		}
		for _, obj := range objects {
			if _, ok := seen[obj.Key]; ok {
				continue
			}
			seen[obj.Key] = struct{}{}
			allObjects = append(allObjects, struct{ Key string }{Key: obj.Key})
		}
	}

	hasMaster := false
	// v0=480p, v1=720p, v2=1080p
	qualityMap := map[string]string{"v0": "480p", "v1": "720p", "v2": "1080p"}
	availableQualities := []fiber.Map{}
	q := tok.Query()

	for _, obj := range allObjects {
		if path.Base(obj.Key) == "master.m3u8" {
			hasMaster = true
		}
		for dir, label := range qualityMap {
			if strings.Contains(obj.Key, "/"+dir+"/") && path.Base(obj.Key) == "index.m3u8" {
				availableQualities = append(availableQualities, fiber.Map{
					"id":       dir,
					"label":    label,
					"playlist": "/api/hls/" + id + "/" + dir + "/index.m3u8?" + q,
				})
			}
		}
	}

	// HLS chưa sẵn sàng
	if !hasMaster {
		if up, upErr := h.uploadRepo.GetUploadByID(c.Context(), uploadID); upErr != nil || up == nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{
				"error": "Video not found",
			})
		}
		return c.JSON(fiber.Map{
			"upload_id": id,
			"hls_ready": false,
			"status":    "processing",
			"message":   "Video đang được xử lý, vui lòng quay lại sau ít phút",
		})
	}

	return c.JSON(fiber.Map{
		"upload_id":  id,
		"hls_ready":  true,
		"master_url": "/api/hls/" + id + "/master.m3u8?" + q,
		"qualities":  availableQualities,
		"status":     "ready",
	})
}
