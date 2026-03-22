package handler

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/service"
)

type UploadHandler struct {
	uploadService service.UploadServiceInterface
}

func NewUploadHandler(uploadService service.UploadServiceInterface) *UploadHandler {
	return &UploadHandler{uploadService: uploadService}
}

// UploadImage handles image upload to MinIO
// POST /api/upload
// Form fields: file (required), folder (optional, default: "general")
func (h *UploadHandler) UploadImage(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "file is required",
		})
	}

	// Get optional folder parameter, default to "general"
	folder := c.FormValue("folder", "general")

	result, err := h.uploadService.UploadImage(c.Context(), file, folder)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Upload successful",
		"data":    result,
	})
}

// Upload handles any file upload (image or video) to MinIO
// POST /api/upload/any
// Form fields: file (required), folder (optional, default: "general")
func (h *UploadHandler) Upload(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "file is required",
		})
	}

	folder := c.FormValue("folder", "general")

	result, err := h.uploadService.Upload(c.Context(), file, folder)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Upload successful",
		"data":    result,
	})
}

// DeleteFile handles file deletion from MinIO
// DELETE /api/upload?url=...
func (h *UploadHandler) DeleteFile(c *fiber.Ctx) error {
	fileURL := c.Query("url")
	if fileURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "url query parameter is required",
		})
	}

	err := h.uploadService.DeleteByURL(c.Context(), fileURL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "File deleted successfully",
	})
}
