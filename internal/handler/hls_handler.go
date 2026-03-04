package handler

import (
	"net/http"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/storage"
)

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

// GetSegment phục vụ từng .ts segment
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
	url, err := h.minioClient.GetPresignedDownloadURLSimple(objectKey, 3600)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate segment URL"})
	}
	return c.Redirect(url, http.StatusFound)
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
