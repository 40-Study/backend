package handler

import (
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/storage"
)

// Rate limit: 10 MB/s for video streaming
const streamRateLimitBytesPerSec = 10 * 1024 * 1024

type HLSHandler struct {
	minioClient *storage.MinioClient
	uploadRepo  repository.VideoUploadRepositoryInterface
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

// getOriginalVideoURL returns relative API URL for streaming original video
func (h *HLSHandler) getOriginalVideoURL(c *fiber.Ctx, uploadID string) (string, error) {
	uid, err := uuid.Parse(uploadID)
	if err != nil {
		return "", err
	}
	// Verify upload exists
	_, err = h.uploadRepo.GetUploadByID(c.Context(), uid)
	if err != nil {
		return "", err
	}
	// Return relative URL - will be served by StreamOriginalVideo endpoint
	return "/api/hls/" + uploadID + "/video.mp4", nil
}

// StreamOriginalVideo streams the original video file (fallback when HLS not ready)
// Route: GET /hls/:upload_id/video.mp4
func (h *HLSHandler) StreamOriginalVideo(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id is required"})
	}

	uid, err := uuid.Parse(uploadID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid upload_id"})
	}

	upload, err := h.uploadRepo.GetUploadByID(c.Context(), uid)
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
	c.Set("Cache-Control", "public, max-age=86400")

	// Stream with rate limiting
	return streamWithRateLimit(c, reader, streamRateLimitBytesPerSec)
}

// GetMasterPlaylist phục vụ master.m3u8
// Route: GET /hls/:upload_id/master.m3u8
// Object key: videos/{upload_id}/master.m3u8
// Nếu HLS chưa sẵn sàng, trả về thông tin fallback để client dùng video gốc
func (h *HLSHandler) GetMasterPlaylist(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id is required"})
	}

	bucket := h.minioClient.GetDefaultBucket()
	objectKey, err := h.resolveExistingHLSObjectKey(c, buildHLSKeyCandidates(uploadID, "master.m3u8"))
	if err != nil {
		// HLS chưa sẵn sàng - trả về fallback URL của video gốc
		originalURL, fallbackErr := h.getOriginalVideoURL(c, uploadID)
		if fallbackErr != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{
				"error": "Video not found",
			})
		}
		return c.Status(http.StatusAccepted).JSON(fiber.Map{
			"hls_ready":    false,
			"fallback_url": originalURL,
			"message":      "HLS đang xử lý, sử dụng video gốc tạm thời",
		})
	}

	url, err := h.minioClient.GetPresignedDownloadURL(c.Context(), bucket, objectKey, time.Hour)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate HLS URL"})
	}
	return c.Redirect(url, http.StatusFound)
}

// GetPlaylist phục vụ playlist của từng quality
// Route: GET /hls/:upload_id/:quality/index.m3u8
// :quality là "v0" (480p), "v1" (720p), "v2" (1080p)
// Object key: videos/{upload_id}/{quality}/index.m3u8
func (h *HLSHandler) GetPlaylist(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	quality := c.Params("quality") // "v0", "v1", "v2"

	if uploadID == "" || quality == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id and quality are required"})
	}

	// Chỉ cho phép v0, v1, v2
	if quality != "v0" && quality != "v1" && quality != "v2" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "quality must be v0, v1, or v2"})
	}

	bucket := h.minioClient.GetDefaultBucket()
	objectKey, err := h.resolveExistingHLSObjectKey(c, buildHLSKeyCandidates(uploadID, quality, "index.m3u8"))
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Playlist not found. Video may still be processing.",
		})
	}

	url, err := h.minioClient.GetPresignedDownloadURL(c.Context(), bucket, objectKey, time.Hour)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate playlist URL"})
	}
	return c.Redirect(url, http.StatusFound)
}

// GetSegment phục vụ từng .ts segment với rate limiting
// Route: GET /hls/:upload_id/:quality/:segment
// :quality là "v0"/"v1"/"v2", :segment là "seg_00001.ts"
// Object key: videos/{upload_id}/{quality}/{segment}
func (h *HLSHandler) GetSegment(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	quality := c.Params("quality")
	segment := c.Params("segment")

	if uploadID == "" || quality == "" || segment == "" {
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
	objectKey, err := h.resolveExistingHLSObjectKey(c, buildHLSKeyCandidates(uploadID, quality, segment))
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
	c.Set("Cache-Control", "public, max-age=31536000") // Cache 1 year

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

// GetVideoInfo trả về thông tin về các HLS stream có sẵn cho video
// Route: GET /hls/:upload_id/info
// Nếu HLS chưa sẵn sàng, trả về fallback_url để client dùng video gốc
func (h *HLSHandler) GetVideoInfo(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id is required"})
	}

	prefixes := []string{
		path.Join("videos", uploadID) + "/",
		path.Join(uploadID) + "/",
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

	for _, obj := range allObjects {
		if path.Base(obj.Key) == "master.m3u8" {
			hasMaster = true
		}
		for dir, label := range qualityMap {
			if strings.Contains(obj.Key, "/"+dir+"/") && path.Base(obj.Key) == "index.m3u8" {
				availableQualities = append(availableQualities, fiber.Map{
					"id":       dir,
					"label":    label,
					"playlist": "/api/hls/" + uploadID + "/" + dir + "/index.m3u8",
				})
			}
		}
	}

	// HLS chưa sẵn sàng - trả về fallback URL của video gốc
	if !hasMaster {
		originalURL, fallbackErr := h.getOriginalVideoURL(c, uploadID)
		if fallbackErr != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{
				"error": "Video not found",
			})
		}
		return c.JSON(fiber.Map{
			"upload_id":    uploadID,
			"hls_ready":    false,
			"fallback_url": originalURL,
			"status":       "processing",
			"message":      "HLS đang xử lý, sử dụng video gốc tạm thời",
		})
	}

	return c.JSON(fiber.Map{
		"upload_id":  uploadID,
		"hls_ready":  true,
		"master_url": "/api/hls/" + uploadID + "/master.m3u8",
		"qualities":  availableQualities,
		"status":     "ready",
	})
}
