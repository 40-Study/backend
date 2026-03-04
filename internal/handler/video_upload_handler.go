package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type VideoUploadHandlerInterface interface {
	InitVideoUpload(c *fiber.Ctx) error
	GetPresignedURLs(c *fiber.Ctx) error
	CompleteChunkUpload(c *fiber.Ctx) error
	CompleteVideoUpload(c *fiber.Ctx) error
	GetUploadStatus(c *fiber.Ctx) error
	GetResumeInfo(c *fiber.Ctx) error
	AbortUpload(c *fiber.Ctx) error
	GetIncompleteUploads(c *fiber.Ctx) error
	GetProcessingQueue(c *fiber.Ctx) error
}

// VideoUploadHandler - Handler xử lý các API endpoints cho video upload
// Hỗ trợ multipart upload với resume capability cross-device
type VideoUploadHandler struct {
	videoUploadService service.VideoUploadServiceInterface
}

func NewVideoUploadHandler(videoUploadService service.VideoUploadServiceInterface) *VideoUploadHandler {
	return &VideoUploadHandler{
		videoUploadService: videoUploadService,
	}
}

// InitVideoUpload - POST /api/videos/upload/init
// Khởi tạo upload session mới
// Flow:
// 1. Parse request body
// 2. Validate required fields (filename, file_size, content_type)
// 3. Lấy user_id từ JWT middleware
// 4. Gọi service để khởi tạo upload
// 5. Trả về upload_id, chunk_size, total_chunks
func (h *VideoUploadHandler) InitVideoUpload(c *fiber.Ctx) error {
	var req dto.InitVideoUploadRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Log request for debugging
	log.Printf("[InitVideoUpload] Request: file=%s, size=%d, type=%s, resource_type=%s, resource_id=%s",
		req.OriginalFileName, req.FileSize, req.ContentType, req.ResourceType, req.ResourceID)

	// Basic validation
	if req.OriginalFileName == "" || req.FileSize == 0 || req.ContentType == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Missing required fields: filename, file_size, content_type",
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		log.Printf("[InitVideoUpload] ERROR: User not authenticated - user_id not found in context")
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "User not authenticated",
		})
	}

	log.Printf("[InitVideoUpload] Authenticated user_id: %s", userID)

	response, err := h.videoUploadService.InitVideoUpload(c.Context(), &req, userID)
	if err != nil {
		log.Printf("[InitVideoUpload] ERROR: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to initiate upload",
			"details": err.Error(),
		})
	}

	log.Printf("[InitVideoUpload] SUCCESS: upload_id=%s", response.UploadID)

	return c.Status(201).JSON(fiber.Map{
		"success": true,
		"data":    response,
	})
}

// GetPresignedURLs handles POST /api/videos/upload/presigned
func (h *VideoUploadHandler) GetPresignedURLs(c *fiber.Ctx) error {
	var req dto.GetPresignedURLsRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Basic validation
	if req.UploadID == uuid.Nil || len(req.ChunkNumbers) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Missing required fields: upload_id, chunk_numbers",
		})
	}

	// Get user ID from JWT middleware
	_, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "User not authenticated",
		})
	}

	response, err := h.videoUploadService.GetPresignedURLs(c.Context(), &req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate presigned URL",
			"details": err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"data":    response,
	})
}

// CompleteChunkUpload handles POST /api/videos/upload/chunk/complete
func (h *VideoUploadHandler) CompleteChunkUpload(c *fiber.Ctx) error {
	var req dto.CompleteChunkUploadRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Get user ID from JWT middleware
	_, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "User not authenticated",
		})
	}

	result, err := h.videoUploadService.CompleteChunkUpload(c.Context(), &req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to complete chunk upload",
			"details": err.Error(),
		})
	}

	return c.Status(200).JSON(result)
}

// CompleteVideoUpload handles POST /api/videos/upload/complete
func (h *VideoUploadHandler) CompleteVideoUpload(c *fiber.Ctx) error {
	var req dto.CompleteVideoUploadRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Basic validation
	if req.UploadID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Missing required field: upload_id",
		})
	}

	// Get user ID from JWT middleware
	_, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "User not authenticated",
		})
	}

	response, err := h.videoUploadService.CompleteVideoUpload(c.Context(), &req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to complete upload",
			"details": err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"data":    response,
	})
}

// AbortUpload handles POST /api/videos/upload/abort
func (h *VideoUploadHandler) AbortUpload(c *fiber.Ctx) error {
	var req dto.AbortUploadRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Get user ID from JWT middleware
	_, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "User not authenticated",
		})
	}
	uploadIdTemp := c.Params("upload_id")

	if uploadIdTemp == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Missing required field: upload_id",
		})
	}

	uploadId, err := uuid.Parse(uploadIdTemp)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid upload_id format",
			"details": err.Error(),
		})
	}


	_, delErr := h.videoUploadService.AbortUpload(c.Context(), &req, uploadId)
	if delErr != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to abort upload",
			"details": delErr.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"message": "Upload aborted successfully",
	})
}

// GetUploadStatus handles GET /api/videos/upload/status/{upload_id}
func (h *VideoUploadHandler) GetUploadStatus(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	if uploadID == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Upload ID is required",
		})
	}

	// Get user ID from JWT middleware
	_, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "User not authenticated",
		})
	}

	uploadIDUUID, err := uuid.Parse(uploadID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid upload ID format",
		})
	}

	status, err := h.videoUploadService.GetUploadStatus(c.Context(), uploadIDUUID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get upload status",
			"details": err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"data":    status,
	})
}

// GetResumeInfo handles GET /api/videos/upload/resume/{upload_id}
func (h *VideoUploadHandler) GetResumeInfo(c *fiber.Ctx) error {
	uploadID := c.Params("upload_id")
	if uploadID == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Upload ID is required",
		})
	}

	// Get user ID from JWT middleware
	_, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "User not authenticated",
		})
	}

	uploadIDUUID, err := uuid.Parse(uploadID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid upload ID format",
		})
	}

	resumeInfo, err := h.videoUploadService.GetResumeInfo(c.Context(), uploadIDUUID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get resume information",
			"details": err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"data":    resumeInfo,
	})
}

// GetIncompleteUploads handles GET /api/videos/upload/incomplete
func (h *VideoUploadHandler) GetIncompleteUploads(c *fiber.Ctx) error {
	user_id := c.Locals("user_id")

	incompleteUploads, err := h.videoUploadService.GetIncompleteUploads(c.Context(), user_id.(uuid.UUID))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get incomplete uploads",
			"details": err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"data":    incompleteUploads,
	})
}

// GetProcessingQueue handles GET /api/videos/queue/processing
func (h *VideoUploadHandler) GetProcessingQueue(c *fiber.Ctx) error {
	user_id := c.Locals("user_id")

	processingJobs, err := h.videoUploadService.GetProcessingQueue(c.Context(), user_id.(uuid.UUID))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get processing queue",
			"details": err.Error(),
		})
	}

	return c.Status(200).JSON(fiber.Map{
		"success": true,
		"data":    processingJobs,
	})
}
