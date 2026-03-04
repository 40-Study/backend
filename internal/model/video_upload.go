package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VideoUploadStatus - Trạng thái của quá trình upload và xử lý video
type VideoUploadStatus string

const (
	VideoUploadStatusInitialized VideoUploadStatus = "initialized" // Session đã tạo, chuẩn bị upload
	VideoUploadStatusUploading   VideoUploadStatus = "uploading"   // Đang upload chunks
	VideoUploadStatusUploaded    VideoUploadStatus = "uploaded"    // Tất cả chunks đã upload xong, multipart completed
	VideoUploadStatusProcessing  VideoUploadStatus = "processing"  // Worker đang xử lý (FFmpeg, thumbnail, HLS)
	VideoUploadStatusReady       VideoUploadStatus = "ready"       // Sẵn sàng xem
	VideoUploadStatusFailed      VideoUploadStatus = "failed"      // Xử lý thất bại
	VideoUploadStatusAborted     VideoUploadStatus = "aborted"     // Đã hủy
)

// VideoUpload - Model lưu trạng thái upload session trong PostgreSQL
// Hỗ trợ resume cross-device và track progress
type VideoUpload struct {
	ID           uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	UserID       uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	ResourceID   uuid.UUID `json:"resource_id" gorm:"type:uuid;not null;index"`    // ID của resource liên kết (ví dụ: blog post, product)
	ResourceType string    `json:"resource_type" gorm:"type:varchar(50);not null"` // Loại resource: 'video', 'blog', 'product'

	// Dữ liệu upload session
	S3UploadKey string `json:"s3_upload_key" gorm:"type:varchar(255);not null;unique_index;column:s3_upload_key"` // S3 MultipartUpload ID (uploadKey từ MinIO)
	ObjectKey   string `json:"object_key" gorm:"type:varchar(500);not null"`                                      // S3 object key (path trong bucket)
	Bucket      string `json:"bucket" gorm:"type:varchar(100);not null"`                                          // MinIO bucket name

	// Metadata file
	OriginalFileName string `json:"original_file_name" gorm:"type:varchar(255);not null"`
	ContentType      string `json:"content_type" gorm:"type:varchar(100);not null"`
	FileSize         int64  `json:"file_size" gorm:"not null"`                   // Tổng dung lượng file (bytes)
	ChunkSize        int64  `json:"chunk_size" gorm:"not null;default:10485760"` // Kích thước mỗi chunk (10MB mặc định)
	TotalChunks      int    `json:"total_chunks" gorm:"not null"`                // Tổng số chunks
	UploadedChunks   int    `json:"uploaded_chunks" gorm:"default:0"`            // Số chunks đã upload (track progress)

	// Upload status and metadata
	Status   VideoUploadStatus `json:"status" gorm:"type:varchar(20);not null;default:'initialized'"`
	Progress float64           `json:"progress" gorm:"type:decimal(5,2);default:0.00"` // 0.00 to 100.00

	// Processing metadata (populated by worker)
	ProcessingStartedAt   *time.Time `json:"processing_started_at,omitempty"`
	ProcessingCompletedAt *time.Time `json:"processing_completed_at,omitempty"`
	ThumbnailURL          *string    `json:"thumbnail_url,omitempty"`
	HLSMasterURL          *string    `json:"hls_master_url,omitempty" gorm:"type:varchar(500)"` // Object key của HLS master playlist trên MinIO
	Duration              *float64   `json:"duration,omitempty"`                                // Video duration in seconds
	Resolution            *string    `json:"resolution,omitempty"`                              // e.g., "1920x1080"
	Bitrate               *int64     `json:"bitrate,omitempty"`                                 // in bps

	// Error handling
	ErrorMessage *string    `json:"error_message,omitempty"`
	RetryCount   int        `json:"retry_count" gorm:"default:0"`
	LastRetryAt  *time.Time `json:"last_retry_at,omitempty"`

	// Cleanup metadata
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // For cleanup incomplete uploads

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// VideoUploadPart - Lưu ETags của từng part sau khi upload hoàn tất (flush từ Redis → DB)
// Chỉ được INSERT khi CompleteVideoUpload thành công, không track realtime trong quá trình upload
// Composite PK (upload_id, part_number) - không cần UUID riêng
type VideoUploadPart struct {
	UploadID   uuid.UUID `json:"upload_id" gorm:"type:uuid;not null;primaryKey"`
	PartNumber int       `json:"part_number" gorm:"not null;primaryKey"`
	ETag       string    `json:"etag" gorm:"type:varchar(255);not null"` // S3 ETag - cần để verify integrity

	// Relationship
	Upload VideoUpload `json:"upload,omitempty" gorm:"foreignKey:UploadID;references:ID"`
}

func (v *VideoUpload) BeforeCreate(tx *gorm.DB) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return nil
}

// TableName returns the table name for VideoUpload
func (VideoUpload) TableName() string {
	return "video_uploads"
}

// TableName returns the table name for VideoUploadPart
func (VideoUploadPart) TableName() string {
	return "video_upload_parts"
}

// IsCompleted checks if all chunks are uploaded
func (v *VideoUpload) IsCompleted() bool {
	return v.UploadedChunks >= v.TotalChunks && v.Status == VideoUploadStatusUploaded
}

// CanResume checks if upload can be resumed
func (v *VideoUpload) CanResume() bool {
	return v.Status == VideoUploadStatusInitialized || v.Status == VideoUploadStatusUploading
}

// UpdateProgress recalculates and updates progress percentage
func (v *VideoUpload) UpdateProgress() {
	if v.TotalChunks > 0 {
		v.Progress = float64(v.UploadedChunks) / float64(v.TotalChunks) * 100.0
	}
}
