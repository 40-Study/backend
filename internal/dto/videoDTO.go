package dto

import (
	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// InitVideoUploadRequest - Request to initialize multipart upload
type InitVideoUploadRequest struct {
	ResourceID       uuid.UUID `json:"resource_id" validate:"required"`
	ResourceType     string    `json:"resource_type" validate:"required,oneof=blog user product course video"`
	OriginalFileName string    `json:"original_file_name" validate:"required"`
	ContentType      string    `json:"content_type" validate:"required"`
	FileSize         int64     `json:"file_size" validate:"required,min=1"`
	ChunkSize        int64     `json:"chunk_size,omitempty"` // Optional, defaults to 10MB
}

// InitVideoUploadResponse - Response from initialize upload
type InitVideoUploadResponse struct {
	UploadID         uuid.UUID `json:"upload_id"`
	UploadKey        string    `json:"upload_key"` // S3 multipart upload ID
	ObjectKey        string    `json:"object_key"` // S3 object key
	ChunkSize        int64     `json:"chunk_size"`
	TotalChunks      int       `json:"total_chunks"`
	ExpiresAt        int64     `json:"expires_at"`                  // Unix timestamp
	DuplicateWarning string    `json:"duplicate_warning,omitempty"` // Warning if duplicate file found
}

// GetPresignedURLsRequest - Request for presigned URLs for chunks
type GetPresignedURLsRequest struct {
	UploadID     uuid.UUID `json:"upload_id" validate:"required"`
	ChunkNumbers []int     `json:"chunk_numbers" validate:"required,dive,min=1"` // Array of chunk numbers to get URLs for
}

// ChunkPresignedURL - Presigned URL info for a specific chunk
type ChunkPresignedURL struct {
	ChunkNumber int    `json:"chunk_number"`
	URL         string `json:"url"`
	ExpiresAt   int64  `json:"expires_at"` // Unix timestamp
}

type GetPresignedURLsResponse struct {
	UploadID uuid.UUID           `json:"upload_id"`
	URLs     []ChunkPresignedURL `json:"urls"`
}

// CompleteChunkUploadRequest - Notify backend that a chunk was uploaded
type CompleteChunkUploadRequest struct {
	UploadID    uuid.UUID `json:"upload_id" validate:"required"`
	ChunkNumber int       `json:"chunk_number" validate:"required,min=1"`
	ETag        string    `json:"etag" validate:"required"`
	Size        int64     `json:"size" validate:"required,min=1"`
}

// CompleteChunkUploadResponse - Response after chunk completion
type CompleteChunkUploadResponse struct {
	Success        bool    `json:"success"`
	ChunkNumber    int     `json:"chunk_number"`
	Progress       float64 `json:"progress"` // Overall upload progress (0-100)
	UploadedChunks int     `json:"uploaded_chunks"`
	TotalChunks    int     `json:"total_chunks"`
	IsCompleted    bool    `json:"is_completed"` // True if all chunks uploaded
}

// CompleteVideoUploadRequest - Request to complete the entire upload
type CompleteVideoUploadRequest struct {
	UploadID uuid.UUID `json:"upload_id" validate:"required"`
}

// CompleteVideoUploadResponse - Response after completing upload
type CompleteVideoUploadResponse struct {
	Success   bool                    `json:"success"`
	UploadID  uuid.UUID               `json:"upload_id"`
	Status    model.VideoUploadStatus `json:"status"`
	ObjectKey string                  `json:"object_key"`
	Message   string                  `json:"message"`
}

// GetUploadStatusResponse - Current status of upload
type GetUploadStatusResponse struct {
	UploadID       uuid.UUID               `json:"upload_id"`
	Status         model.VideoUploadStatus `json:"status"`
	Progress       float64                 `json:"progress"`
	UploadedChunks int                     `json:"uploaded_chunks"`
	TotalChunks    int                     `json:"total_chunks"`
	FileSize       int64                   `json:"file_size"`

	// Processing info (if available)
	ProcessingStartedAt   *int64   `json:"processing_started_at,omitempty"`   // Unix timestamp
	ProcessingCompletedAt *int64   `json:"processing_completed_at,omitempty"` // Unix timestamp
	ThumbnailURL          *string  `json:"thumbnail_url,omitempty"`
	Duration              *float64 `json:"duration,omitempty"`
	Resolution            *string  `json:"resolution,omitempty"`

	// Error info (if failed)
	ErrorMessage *string `json:"error_message,omitempty"`
	RetryCount   int     `json:"retry_count"`

	CreatedAt int64 `json:"created_at"` // Unix timestamp
	UpdatedAt int64 `json:"updated_at"` // Unix timestamp
}

// GetResumeInfoResponse - Information needed to resume upload
type GetResumeInfoResponse struct {
	UploadID        uuid.UUID `json:"upload_id"`
	MediaID         uuid.UUID `json:"media_id"`
	ObjectKey       string    `json:"object_key"`
	FileName        string    `json:"file_name"`
	FileSize        int64     `json:"file_size"`
	CanResume       bool      `json:"can_resume"`
	CompletedChunks []int     `json:"completed_chunks"` // List of chunk numbers already uploaded
	PendingChunks   []int     `json:"pending_chunks"`   // List of chunk numbers still needed
	ChunkSize       int64     `json:"chunk_size"`
	TotalChunks     int       `json:"total_chunks"`
	Progress        float64   `json:"progress"`
}

// AbortUploadRequest - Request to abort/cancel upload
type AbortUploadRequest struct {
	Reason   string    `json:"reason,omitempty"` // Optional reason for abort
}

// AbortUploadResponse - Response after aborting upload
type AbortUploadResponse struct {
	Success  bool      `json:"success"`
	UploadID uuid.UUID `json:"upload_id"`
	Message  string    `json:"message"`
}

// Type aliases for handler compatibility
type InitiateUploadRequest = InitVideoUploadRequest
type GetPresignedURLRequest = GetPresignedURLsRequest
type CompleteChunkRequest = CompleteChunkUploadRequest
type CompleteUploadRequest = CompleteVideoUploadRequest
