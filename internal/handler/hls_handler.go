package handler

import (
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/storage"
)

// Rate limit: 10 MB/s for video streaming
const streamRateLimitBytesPerSec = 10 * 1024 * 1024

type HLSHandler struct {
	minioClient *storage.MinioClient
}

func NewHLSHandler(minioClient *storage.MinioClient) *HLSHandler {
	return &HLSHandler{minioClient: minioClient}
}

// GetMasterPlaylist phục vụ master.m3u8
// Route: GET /hls/:upload_id/master.m3u8
// Object key: videos/{upload_id}/master.m3u8
func (h *HLSHandler) GetMasterPlaylist(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id is required"})
	}

	objectKey := path.Join("videos", uploadID, "master.m3u8")
	url, err := h.minioClient.GetPresignedDownloadURLSimple(objectKey, 3600)
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

	objectKey := path.Join("videos", uploadID, quality, "index.m3u8")
	url, err := h.minioClient.GetPresignedDownloadURLSimple(objectKey, 3600)
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

	objectKey := path.Join("videos", uploadID, quality, segment)

	// Stream with rate limiting instead of redirect
	reader, err := h.minioClient.GetObject(c.Context(), "videos", objectKey)
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
func (h *HLSHandler) GetVideoInfo(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "upload_id is required"})
	}

	// Kiểm tra master.m3u8 có tồn tại không
	prefix := path.Join("videos", uploadID) + "/"
	objects, err := h.minioClient.ListObjectsWithPrefix(prefix)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list HLS files"})
	}

	hasMaster := false
	// v0=480p, v1=720p, v2=1080p
	qualityMap := map[string]string{"v0": "480p", "v1": "720p", "v2": "1080p"}
	availableQualities := []fiber.Map{}

	for _, obj := range objects {
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

	if !hasMaster {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "HLS files not found. Video may still be processing.",
		})
	}

	return c.JSON(fiber.Map{
		"upload_id":  uploadID,
		"master_url": "/api/hls/" + uploadID + "/master.m3u8",
		"qualities":  availableQualities,
		"status":     "ready",
	})
}
