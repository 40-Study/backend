package queue

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"

	"github.com/google/uuid"
)

type QueueMessage interface {
	GetID() string
	GetResourceID() uuid.UUID
	GetUserID() uuid.UUID
	GetCreatedAt() time.Time
	Validate() error
}

type ProcessResult interface {
	GetID() string
	GetStatus() string
	GetError() string
	IsSuccess() bool
}

type MessageProcessor interface {
	Process(ctx context.Context, message QueueMessage) (ProcessResult, error)
	Cleanup(message QueueMessage) error
}

type MediaType string

const (
	MediaImage MediaType = "image"
	MediaVideo MediaType = "video"
)

type VideoProcessingMessage struct {
	UploadID     uuid.UUID `json:"upload_id"`     // Video upload UUID
	ObjectKey    string    `json:"object_key"`    // S3/MinIO object key
	Bucket       string    `json:"bucket"`        // S3/MinIO bucket name
	UserID       uuid.UUID `json:"user_id"`       // User who uploaded
	ResourceID   uuid.UUID `json:"resource_id"`   // Associated resource
	ResourceType string    `json:"resource_type"` // blog, user, product, etc.
	ContentType  string    `json:"content_type"`  // video/mp4, etc.
	FileSize     int64     `json:"file_size"`     // File size in bytes
	ETag         string    `json:"etag"`          // S3 ETag from completed upload
	RetryCount   int       `json:"retry_count"`   // Current retry attempt
	CreatedAt    int64     `json:"created_at"`    // Unix timestamp

	ProcessingOptions VideoProcessingOptions `json:"processing_options,omitempty"`
}

func (m *VideoProcessingMessage) Validate() error {
	if m.UploadID == uuid.Nil {
		return fmt.Errorf("upload ID is required")
	}
	if m.ObjectKey == "" {
		return fmt.Errorf("object key is required")
	}
	if m.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if m.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}
	return nil
}

type VideoProcessingOptions struct {
	GenerateThumbnail bool     `json:"generate_thumbnail"` // Generate video thumbnail
	ScanForViruses    bool     `json:"scan_for_viruses"`   // Virus scan
	EncodeProfiles    []string `json:"encode_profiles"`    // Video encoding profiles (480p, 720p, 1080p)
	ExtractMetadata   bool     `json:"extract_metadata"`   // Extract duration, resolution, etc.
	GeneratePreview   bool     `json:"generate_preview"`   // Generate preview clips
}

type VideoProcessingResult struct {
	UploadID    uuid.UUID              `json:"upload_id"`
	Success     bool                   `json:"success"`
	ErrorMsg    *string                `json:"error_msg,omitempty"`
	Results     VideoProcessingOutputs `json:"results,omitempty"`
	ProcessedAt int64                  `json:"processed_at"` // Unix timestamp
}

type VideoProcessingOutputs struct {
	ThumbnailURL *string                `json:"thumbnail_url,omitempty"`
	HLSMasterURL *string                `json:"hls_master_url,omitempty"` // Object key của HLS master playlist trên MinIO
	Duration     *float64               `json:"duration,omitempty"`       // seconds
	Resolution   *string                `json:"resolution,omitempty"`     // "1920x1080"
	Bitrate      *int64                 `json:"bitrate,omitempty"`        // bps
	EncodedFiles map[string]string      `json:"encoded_files,omitempty"`  // profile -> object_key
	PreviewURL   *string                `json:"preview_url,omitempty"`
	VirusScanOk  *bool                  `json:"virus_scan_ok,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type CleanupMessage struct {
	TaskType       string `json:"task_type"` // "expired_uploads", "incomplete_uploads"
	OlderThanHours int    `json:"older_than_hours,omitempty"`
	CreatedAt      int64  `json:"created_at"`
}

type MediaStatus string

const (
	StatusPending    MediaStatus = "pending"
	StatusProcessing MediaStatus = "processing"
	StatusCompleted  MediaStatus = "completed"
	StatusFailed     MediaStatus = "failed"
)

type UploadMediaMessage struct {
	ID          string    `json:"id"`           // UUID v4 - message tracking
	ResourceID  uuid.UUID `json:"resource_id"`  // Resource ID (Blog, User, Product, etc.)
	UserID      uuid.UUID `json:"user_id"`      // User upload
	MediaType   MediaType `json:"media_type"`   // image | video
	FileName    string    `json:"file_name"`    // Original filename
	TempPath    string    `json:"temp_path"`    // Temp file path (or shared volume)
	ContentType string    `json:"content_type"` // MIME type
	Size        int64     `json:"size"`         // File size (bytes)
	Folder      string    `json:"folder"`       // Target folder (e.g. "blogs")
	CreatedAt   time.Time `json:"created_at"`   // Message created time
}

// GetID - Implement QueueMessage interface
func (m *UploadMediaMessage) GetID() string {
	return m.ID
}

// GetResourceID - Implement QueueMessage interface
func (m *UploadMediaMessage) GetResourceID() uuid.UUID {
	return m.ResourceID
}

// GetUserID - Implement QueueMessage interface
func (m *UploadMediaMessage) GetUserID() uuid.UUID {
	return m.UserID
}

// GetCreatedAt - Implement QueueMessage interface
func (m *UploadMediaMessage) GetCreatedAt() time.Time {
	return m.CreatedAt
}

// Validate - Implement QueueMessage interface
// Kiểm tra message có hợp lệ không
func (m *UploadMediaMessage) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("message ID is required")
	}
	if m.ResourceID == uuid.Nil {
		return fmt.Errorf("resource ID is required")
	}
	if m.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}
	if m.MediaType == "" {
		return fmt.Errorf("media type is required")
	}
	if m.FileName == "" {
		return fmt.Errorf("file name is required")
	}
	if m.TempPath == "" {
		return fmt.Errorf("temp path is required")
	}
	return nil
}

//
// ===== CLIENT REQUEST / RESPONSE =====
//

// BlogMediaUploadRequest là request từ client
type BlogMediaUploadRequest struct {
	ResourceID uuid.UUID               `form:"resource_id" validate:"required"`
	MediaType  MediaType               `form:"media_type" validate:"required,oneof=image video"`
	Files      []*multipart.FileHeader `form:"files" validate:"required,min=1"`
}

// BlogMediaUploadResponse trả về cho client
type BlogMediaUploadResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	QueuedIDs []string `json:"queued_ids"` // UUID list
	Count     int      `json:"count"`
}

//
// ===== STATUS TRACKING =====
//

// MediaProcessingStatus để client check trạng thái
type MediaProcessingStatus struct {
	ID         string                `json:"id"`
	ResourceID uuid.UUID             `json:"resource_id"`
	Status     MediaStatus           `json:"status"`   // pending | processing | completed | failed
	Progress   float64               `json:"progress"` // 0.0 - 1.0
	Result     *ProcessedMediaResult `json:"result,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

//
// ===== VIDEO PROCESSING MODELS =====
//

type VideoProcessingVariants struct {
	HLS        *VideoHLSVariant    `json:"hls,omitempty"`
	Poster     *VideoPosterVariant `json:"poster,omitempty"`
	Thumbnails []VideoThumbnail    `json:"thumbnails,omitempty"`
}

type VideoHLSVariant struct {
	MasterURL string                `json:"master_url"`
	Variants  []VideoQualityVariant `json:"variants"`
}

type VideoQualityVariant struct {
	Quality     string `json:"quality"` // 1080p, 720p...
	URL         string `json:"url"`     // m3u8 URL
	Height      int32  `json:"height"`
	BitrateKbps int32  `json:"bitrate_kbps"`
	IsDefault   bool   `json:"is_default"`
}

type VideoPosterVariant struct {
	URL    string `json:"url"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
}

type VideoThumbnail struct {
	URL       string  `json:"url"`
	Timestamp float64 `json:"timestamp"` // seconds
	Width     int32   `json:"width"`
	Height    int32   `json:"height"`
}

//
// ===== IMAGE PROCESSING MODELS =====
//

type ImageProcessingVariants struct {
	Original   *ImageVariant            `json:"original"`
	Thumbnails map[string]*ImageVariant `json:"thumbnails"` // e.g. "256", "1024"
}

type ImageVariant struct {
	URL    string `json:"url"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
	Size   int64  `json:"size"`
	Format string `json:"format"` // webp, jpg...
}

type MediaUploadStats struct {
	TotalProcessed  int64     `json:"total_processed"`
	TotalFailed     int64     `json:"total_failed"`
	AverageTime     float64   `json:"average_time"` // seconds (e.g. 2.1)
	ProcessedToday  int64     `json:"processed_today"`
	QueueSize       int64     `json:"queue_size"`
	LastProcessedAt time.Time `json:"last_processed_at"`
}
