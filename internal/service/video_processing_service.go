package service

import (
	"context"
	"io"
	"log"
	"time"

	"study.com/v1/internal/config"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/storage"
)

// MinioClientAdapter adapts MinioClient to VideoStorageInterface
type MinioClientAdapter struct {
	storage *storage.MinioClient
	config  *config.Config
}

func NewMinioClientAdapter(storage *storage.MinioClient, config *config.Config) *MinioClientAdapter {
	return &MinioClientAdapter{storage: storage, config: config}
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
	processor     *rabbitmq_queue.VideoProcessor
	queueSetup    *rabbitmq_queue.VideoQueueSetup
	uploadRepo    repository.VideoUploadRepositoryInterface
	uploadService VideoUploadServiceInterface
	workerCtx     context.Context
	workerCancel  context.CancelFunc
}

func NewVideoProcessingService(
	uploadRepo repository.VideoUploadRepositoryInterface,
	storage *storage.MinioClient,
	uploadService VideoUploadServiceInterface,
	rabbitMQService *rabbitmq_queue.RabbitMQService,
	cfg *config.Config,
) (*VideoProcessingService, error) {
	// Initialize queue setup reusing existing RabbitMQ connection
	queueSetup := rabbitmq_queue.NewVideoQueueSetup(rabbitMQService)

	// Setup video queues
	err := queueSetup.SetupVideoQueues()
	if err != nil {
		return nil, err
	}

	// Create storage adapter for video processor
	storageAdapter := &MinioClientAdapter{storage: storage, config: cfg}

	// Lấy temp dir từ config, mặc định là /tmp/video-processing
	tempDir := cfg.TempDir
	if tempDir == "" {
		tempDir = "/tmp/video-processing"
	}

	// Initialize video processor
	processor := rabbitmq_queue.NewVideoProcessor(
		uploadRepo,     // Interface type
		storageAdapter, // Adapter for storage interface
		"ffmpeg",       // ffmpeg path
		tempDir,        // temp directory for processing
		false,          // clamav disabled for now
		3,              // max retries
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
		func(message rabbitmq_queue.VideoProcessingMessage) error {
			log.Printf("Processing video for upload: %s", message.UploadID)

			// Set default processing options if not provided
			if message.ProcessingOptions.GenerateThumbnail == false &&
				message.ProcessingOptions.ScanForViruses == false &&
				len(message.ProcessingOptions.EncodeProfiles) == 0 {
				message.ProcessingOptions = rabbitmq_queue.VideoProcessingOptions{
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
	cleanupMessage := rabbitmq_queue.CleanupMessage{
		TaskType:       "scheduled_cleanup",
		OlderThanHours: 24,
		CreatedAt:      time.Now().Unix(),
	}

	if err := vps.queueSetup.PublishCleanupMessage(ctx, cleanupMessage); err != nil {
		log.Printf("Failed to publish cleanup message: %v", err)
	}

	log.Println("Scheduled cleanup completed")
}
