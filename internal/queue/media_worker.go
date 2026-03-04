package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type MediaProcessor interface {
	ProcessMedia(ctx context.Context, message *UploadMediaMessage) (*ProcessedMediaResult, error)

	// CleanupTempFile - Xóa file tạm sau khi xử lý xong
	CleanupTempFile(tempPath string) error
}

// VideoProcessorInterface - Interface for video processing
type VideoProcessorInterface interface {
	ProcessVideo(ctx context.Context, message VideoProcessingMessage) error
}

// ProcessedMediaResult contains the result of media processing
// Implements ProcessResult interface
type ProcessedMediaResult struct {
	ID             string                 `json:"id"`
	Status         string                 `json:"status"`
	Error          string                 `json:"error,omitempty"`
	OriginalURL    string                 `json:"original_url,omitempty"`
	ProcessedURLs  map[string]string      `json:"processed_urls,omitempty"`
	MediaType      string                 `json:"media_type"`
	ProcessingTime time.Duration          `json:"processing_time"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CompletedAt    time.Time              `json:"completed_at"`
}

// GetID - Implement ProcessResult interface
func (r *ProcessedMediaResult) GetID() string {
	return r.ID
}

// GetStatus - Implement ProcessResult interface
func (r *ProcessedMediaResult) GetStatus() string {
	return r.Status
}

// GetError - Implement ProcessResult interface
func (r *ProcessedMediaResult) GetError() string {
	return r.Error
}

// IsSuccess - Implement ProcessResult interface
func (r *ProcessedMediaResult) IsSuccess() bool {
	return r.Status == "completed" || r.Status == "success"
}

type MediaWorker struct {
	*BaseWorker    // Embed base worker (chứa retry/DLQ logic)
	videoProcessor VideoProcessorInterface
}

// NewMediaWorker - Tạo MediaWorker mới
// rabbitMQService: RabbitMQ service để communicate với queue
// videoProcessor: Service implement VideoProcessorInterface (có thể có repository injected)
func NewMediaWorker(
	rabbitMQService *RabbitMQService,
	videoProcessor VideoProcessorInterface,
) *MediaWorker {
	worker := &MediaWorker{
		videoProcessor: videoProcessor,
	}

	// Tạo BaseWorker với callback handler
	// BaseWorker sẽ lo tất cả retry/DLQ logic
	worker.BaseWorker = NewBaseWorker(
		rabbitMQService,
		WorkerConfig{
			QueueName:    VideoProcessingQueue, // Changed from "media.upload" to use constant
			MaxRetries:   3,
			RetryBackoff: 5 * time.Second,
		},
		worker.handleMessage, // Callback để BaseWorker gọi
	)

	return worker
}

// handleMessage - Callback được BaseWorker gọi khi có message mới
// BaseWorker sẽ lo retry/DLQ, method này CHỈ parse và delegate
func (w *MediaWorker) handleMessage(ctx context.Context, messageData []byte) error {
	// Parse message as VideoProcessingMessage
	var message VideoProcessingMessage
	if err := json.Unmarshal(messageData, &message); err != nil {
		return fmt.Errorf("invalid message format: %w", err)
	}

	// Validate message
	if err := message.Validate(); err != nil {
		return fmt.Errorf("invalid message: %w", err)
	}

	log.Printf("[MediaWorker] ===== NEW JOB RECEIVED =====")
	log.Printf("[MediaWorker] upload_id=%s | object_key=%s | user_id=%s | bucket=%s",
		message.UploadID, message.ObjectKey, message.UserID, message.Bucket)
	log.Printf("[MediaWorker] options: thumbnail=%v | preview=%v | virus_scan=%v | profiles=%v",
		message.ProcessingOptions.GenerateThumbnail,
		message.ProcessingOptions.GeneratePreview,
		message.ProcessingOptions.ScanForViruses,
		message.ProcessingOptions.EncodeProfiles)

	start := time.Now()

	err := w.videoProcessor.ProcessVideo(ctx, message)
	if err != nil {
		log.Printf("[MediaWorker] ===== JOB FAILED after %v | upload_id=%s | error: %v =====",
			time.Since(start), message.UploadID, err)
		return fmt.Errorf("failed to process video: %w", err)
	}

	log.Printf("[MediaWorker] ===== JOB DONE in %v | upload_id=%s =====",
		time.Since(start), message.UploadID)
	return nil
}
