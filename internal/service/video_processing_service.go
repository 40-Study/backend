package service

import (
	"context"
	"io"
	"log"
	"time"

	"study.com/v1/internal/queue"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/storage"
)

// MinioClientAdapter adapts MinioClient to VideoStorageInterface
type MinioClientAdapter struct {
	storage *storage.MinioClient
}

func NewMinioClientAdapter(storage *storage.MinioClient) *MinioClientAdapter {
	return &MinioClientAdapter{storage: storage}
}

func (a *MinioClientAdapter) GetObject(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	return a.storage.GetObject(ctx, bucket, objectKey)
}

func (a *MinioClientAdapter) PutObjectFromFile(ctx context.Context, bucket, objectKey, filePath, contentType string) error {
	return a.storage.PutObjectFromFile(ctx, bucket, objectKey, filePath, contentType)
}

func (a *MinioClientAdapter) GetPresignedDownloadURL(ctx context.Context, bucket, objectKey string, expires time.Duration) (string, error) {
	return a.storage.GetPresignedDownloadURL(ctx, bucket, objectKey, expires)
}

func (a *MinioClientAdapter) UploadHLSDirectory(ctx context.Context, localDir, videoID, bucket string) error {
	return a.storage.UploadHLSDirectory(ctx, localDir, videoID, bucket)
}

type VideoProcessingServiceInterface interface {
	StartWorker(ctx context.Context) error
	StopWorker() error
	StartCleanupScheduler(ctx context.Context) error
}

type VideoProcessingService struct {
	processor     *queue.VideoProcessor
	queueSetup    *queue.VideoQueueSetup
	uploadRepo    repository.VideoUploadRepositoryInterface
	uploadService VideoUploadServiceInterface
	workerCtx     context.Context
	workerCancel  context.CancelFunc
}

func NewVideoProcessingService(
	uploadRepo repository.VideoUploadRepositoryInterface,
	storage *storage.MinioClient,
	uploadService VideoUploadServiceInterface,
	rabbitMQService *queue.RabbitMQService,
) (*VideoProcessingService, error) {
	// Initialize queue setup reusing existing RabbitMQ connection
	queueSetup := queue.NewVideoQueueSetup(rabbitMQService)

	// Setup video queues
	err := queueSetup.SetupVideoQueues()
	if err != nil {
		return nil, err
	}

	// Create storage adapter for video processor
	storageAdapter := &MinioClientAdapter{storage: storage}

	// Initialize video processor
	processor := queue.NewVideoProcessor(
		uploadRepo,              // Interface type
		storageAdapter,          // Adapter for storage interface
		"ffmpeg",                // ffmpeg path
		"/tmp/video-processing", // temp directory for processing
		false,                   // clamav disabled for now
		3,                       // max retries
	)

	return &VideoProcessingService{
		processor:     processor,
		queueSetup:    queueSetup,
		uploadRepo:    uploadRepo,
		uploadService: uploadService,
	}, nil
}

func (vps *VideoProcessingService) StartWorker(ctx context.Context) error {
	vps.workerCtx, vps.workerCancel = context.WithCancel(ctx)

	log.Println("Starting video processing worker...")

	// Start consuming video processing messages
	err := vps.queueSetup.StartVideoProcessingConsumer(
		vps.workerCtx,
		func(message queue.VideoProcessingMessage) error {
			log.Printf("Processing video for upload: %s", message.UploadID)

			// Set default processing options if not provided
			if message.ProcessingOptions.GenerateThumbnail == false &&
				message.ProcessingOptions.ScanForViruses == false &&
				len(message.ProcessingOptions.EncodeProfiles) == 0 {
				message.ProcessingOptions = queue.VideoProcessingOptions{
					GenerateThumbnail: true,
					ScanForViruses:    true,
					ExtractMetadata:   true,
					GeneratePreview:   false,                    // Optional
					EncodeProfiles:    []string{"480p", "720p"}, // Optional multi-resolution
				}
			}

			return vps.processor.ProcessVideo(vps.workerCtx, message)
		},
	)

	if err != nil {
		return err
	}

	log.Println("Video processing worker started successfully")
	return nil
}

func (vps *VideoProcessingService) StopWorker() error {
	if vps.workerCancel != nil {
		vps.workerCancel()
	}

	if vps.queueSetup != nil {
		return vps.queueSetup.Close()
	}

	return nil
}

func (vps *VideoProcessingService) StartCleanupScheduler(ctx context.Context) error {
	log.Println("Starting video upload cleanup scheduler...")

	// Run cleanup every hour
	ticker := time.NewTicker(1 * time.Hour)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Cleanup scheduler shutting down...")
				return
			case <-ticker.C:
				vps.performCleanup(ctx)
			}
		}
	}()

	return nil
}

func (vps *VideoProcessingService) performCleanup(ctx context.Context) {
	log.Println("Performing scheduled cleanup...")

	// Clean up expired uploads (older than 24 hours)
	if err := vps.uploadService.CleanupExpiredUploads(ctx); err != nil {
		log.Printf("Failed to cleanup expired uploads: %v", err)
	}

	// Clean up incomplete uploads older than 72 hours
	if err := vps.uploadService.CleanupIncompleteUploads(ctx, 72); err != nil {
		log.Printf("Failed to cleanup incomplete uploads: %v", err)
	}

	// Publish cleanup message to queue for other cleanup tasks
	cleanupMessage := queue.CleanupMessage{
		TaskType:       "scheduled_cleanup",
		OlderThanHours: 24,
		CreatedAt:      time.Now().Unix(),
	}

	if err := vps.queueSetup.PublishCleanupMessage(ctx, cleanupMessage); err != nil {
		log.Printf("Failed to publish cleanup message: %v", err)
	}

	log.Println("Scheduled cleanup completed")
}
