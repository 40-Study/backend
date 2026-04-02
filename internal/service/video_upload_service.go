package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/storage"
)

// VideoUploadServiceInterface - Interface cho service quản lý video upload
// Hỗ trợ multipart upload với tính năng resume cross-device
type VideoUploadServiceInterface interface {
	// Vòng đời upload
	InitVideoUpload(ctx context.Context, req *dto.InitVideoUploadRequest, userID uuid.UUID) (*dto.InitVideoUploadResponse, error) // Khởi tạo upload session
	GetPresignedURLs(ctx context.Context, req *dto.GetPresignedURLsRequest) (*dto.GetPresignedURLsResponse, error)                // Lấy presigned URLs cho chunks
	CompleteChunkUpload(ctx context.Context, req *dto.CompleteChunkUploadRequest) (*dto.CompleteChunkUploadResponse, error)       // Đánh dấu chunk đã upload xong
	CompleteVideoUpload(ctx context.Context, req *dto.CompleteVideoUploadRequest) (*dto.CompleteVideoUploadResponse, error)       // Hoàn tất upload và bắt đầu xử lý

	// Trạng thái và resume
	GetUploadStatus(ctx context.Context, uploadID uuid.UUID) (*dto.GetUploadStatusResponse, error) // Lấy trạng thái upload hiện tại
	GetResumeInfo(ctx context.Context, uploadID uuid.UUID) (*dto.GetResumeInfoResponse, error)     // Lấy thông tin để resume upload

	// Quản lý
	AbortUpload(ctx context.Context, req *dto.AbortUploadRequest, uploadID uuid.UUID) (*dto.AbortUploadResponse, error) // Hủy upload
	GetIncompleteUploads(ctx context.Context, userID uuid.UUID) ([]model.VideoUpload, error)                            // Lấy danh sách upload chưa hoàn thành
	GetProcessingQueue(ctx context.Context, userID uuid.UUID) ([]model.VideoUpload, error)                              // Lấy danh sách video đang xử lý

	// Dọn dẹp
	CleanupExpiredUploads(ctx context.Context) error                        // Dọn upload đã hết hạn
	CleanupIncompleteUploads(ctx context.Context, olderThanHours int) error // Dọn upload chưa hoàn thành lâu
}

type VideoUploadService struct {
	uploadRepo   repository.VideoUploadRepositoryInterface // Repository để lưu trạng thái upload vào PostgreSQL
	storage      *storage.MinioClient                      // MinIO client để upload file lên S3-compatible storage
	queueService *rabbitmq_queue.RabbitMQService           // RabbitMQ để gửi job xử lý video
	videoQueue   *rabbitmq_queue.VideoQueueSetup           // Direct access to video queue for processing
	redisClient  *redis.Client                             // Redis client để track active uploads (SADD/SREM active_uploads set)
}

func NewVideoUploadService(
	uploadRepo repository.VideoUploadRepositoryInterface,
	storage *storage.MinioClient,
	queueService *rabbitmq_queue.RabbitMQService,
	videoQueue *rabbitmq_queue.VideoQueueSetup,
	redisClient *redis.Client,
) *VideoUploadService {
	return &VideoUploadService{
		uploadRepo:   uploadRepo,
		storage:      storage,
		queueService: queueService,
		videoQueue:   videoQueue,
		redisClient:  redisClient,
	}
}

// InitVideoUpload - Khởi tạo session upload mới
// Flow:
// 1. Validate request
// 2. Check duplicate file (tránh upload trùng)
// 3. Generate object key và tính số chunks
// 4. Khởi tạo multipart upload với MinIO
// 5. Lưu upload record vào PostgreSQL
// 6. Tạo chunk records để track từng chunk
// 7. Track active upload trong Redis Set (SADD active_uploads)
func (s *VideoUploadService) InitVideoUpload(ctx context.Context, req *dto.InitVideoUploadRequest, userID uuid.UUID) (*dto.InitVideoUploadResponse, error) {
	// 1. Validate request
	if err := s.validateInitRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check for duplicate file (same filename + size + user)
	existingUploads, err := s.uploadRepo.FindUploadsByFilename(ctx, userID, req.OriginalFileName, req.FileSize)
	var duplicateWarning string
	if err == nil && len(existingUploads) > 0 {
		completedCount := 0
		for _, upload := range existingUploads {
			if upload.Status == model.VideoUploadStatusReady || upload.Status == model.VideoUploadStatusUploaded {
				completedCount++
			}
		}
		if completedCount > 0 {
			duplicateWarning = fmt.Sprintf("Warning: You have %d completed upload(s) with the same filename and size", completedCount)
			log.Printf("⚠️ Duplicate file warning: user=%s, file=%s, size=%d, existing=%d",
				userID, req.OriginalFileName, req.FileSize, completedCount)
		}
	}

	// Generate object key
	objectKey := s.generateObjectKey(req.ResourceType, req.ResourceID.String(), req.OriginalFileName)

	// Set default chunk size if not provided (25MB)
	chunkSize := req.ChunkSize
	if chunkSize == 0 {
		chunkSize = 25 * 1024 * 1024 // 25MB default
	}

	// Calculate total chunks needed
	totalChunks := int(math.Ceil(float64(req.FileSize) / float64(chunkSize)))

	// Initiate multipart upload with MinIO
	bucket := s.storage.GetDefaultBucket()
	uploadKey, err := s.storage.InitMultipartUpload(ctx, bucket, objectKey, req.ContentType)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate multipart upload: %w", err)
	}

	// Create upload record
	upload := &model.VideoUpload{
		ID:               uuid.New(),
		UserID:           userID,
		ResourceID:       req.ResourceID,
		ResourceType:     req.ResourceType,
		S3UploadKey:      uploadKey,
		ObjectKey:        objectKey,
		Bucket:           bucket,
		OriginalFileName: req.OriginalFileName,
		ContentType:      req.ContentType,
		FileSize:         req.FileSize,
		ChunkSize:        chunkSize,
		TotalChunks:      totalChunks,
		Status:           model.VideoUploadStatusInitialized,
		ExpiresAt:        timePtr(time.Now().Add(24 * time.Hour)), // 24h to complete upload
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	err = s.uploadRepo.CreateUpload(ctx, upload)
	if err != nil {
		// If DB creation fails, abort the multipart upload
		if abortErr := s.storage.AbortMultipartUpload(ctx, bucket, objectKey, uploadKey); abortErr != nil {
			log.Printf("Failed to abort multipart upload after DB error: %v", abortErr)
		}
		return nil, fmt.Errorf("failed to create upload record: %w", err)
	}

	// Update status to indicate ready for uploading
	err = s.uploadRepo.UpdateUploadStatus(ctx, upload.ID, model.VideoUploadStatusUploading, nil)
	if err != nil {
		log.Printf("Failed to update upload status to uploading: %v", err)
	}

	if s.redisClient != nil {
		redisKey := "active_uploads"
		if err := s.redisClient.SAdd(ctx, redisKey, upload.ID.String()).Err(); err != nil {
			log.Printf("Failed to add upload to Redis active set: %v", err)
		} else {
			s.redisClient.Expire(ctx, redisKey, 7*24*time.Hour)
			log.Printf("Đã thêm upload %s vào Redis Set active_uploads", upload.ID)
		}

		// Lưu meta vào Redis Hash để CompleteChunkUpload đọc không cần query DB
		// Key: upload:{id}:meta, field: total_chunks / chunk_size
		redisMetaKey := fmt.Sprintf("upload:%s:meta", upload.ID)
		pipe := s.redisClient.Pipeline()
		pipe.HMSet(ctx, redisMetaKey, map[string]interface{}{
			"total_chunks": totalChunks,
			"chunk_size":   chunkSize,
		})
		pipe.Expire(ctx, redisMetaKey, 7*24*time.Hour)
		if _, pipeErr := pipe.Exec(ctx); pipeErr != nil {
			log.Printf("Failed to store upload meta in Redis: %v", pipeErr)
		}
	}

	return &dto.InitVideoUploadResponse{
		UploadID:         upload.ID,
		UploadKey:        uploadKey,
		ObjectKey:        objectKey,
		ChunkSize:        chunkSize,
		TotalChunks:      totalChunks,
		ExpiresAt:        upload.ExpiresAt.Unix(),
		DuplicateWarning: duplicateWarning,
	}, nil
}

// GetPresignedURLs generates presigned URLs for specific chunks
func (s *VideoUploadService) GetPresignedURLs(ctx context.Context, req *dto.GetPresignedURLsRequest) (*dto.GetPresignedURLsResponse, error) {
	// Get upload record
	upload, err := s.uploadRepo.GetUploadByID(ctx, req.UploadID)
	if err != nil {
		return nil, fmt.Errorf("upload not found: %w", err)
	}

	// Validate upload can still receive chunks
	if !upload.CanResume() {
		return nil, fmt.Errorf("upload cannot be resumed, current status: %s", upload.Status)
	}

	// Check if upload has expired
	if upload.ExpiresAt != nil && time.Now().After(*upload.ExpiresAt) {
		return nil, fmt.Errorf("upload session has expired")
	}

	// Validate chunk numbers
	for _, chunkNum := range req.ChunkNumbers {
		if chunkNum < 1 || chunkNum > upload.TotalChunks {
			return nil, fmt.Errorf("invalid chunk number %d, must be between 1 and %d", chunkNum, upload.TotalChunks)
		}
	}

	// Generate presigned URLs
	urls := make([]dto.ChunkPresignedURL, 0, len(req.ChunkNumbers))
	for _, chunkNum := range req.ChunkNumbers {
		presignedURL, err := s.storage.GetPresignedUploadURL(ctx, upload.Bucket, upload.ObjectKey, upload.S3UploadKey, chunkNum)
		if err != nil {
			log.Printf("Failed to generate presigned URL for chunk %d: %v", chunkNum, err)
			continue // Skip this chunk, don't fail entire request
		}

		urls = append(urls, dto.ChunkPresignedURL{
			ChunkNumber: chunkNum,
			URL:         presignedURL,
			ExpiresAt:   time.Now().Add(time.Hour).Unix(), // 1 hour expiry
		})
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("failed to generate any presigned URLs")
	}

	return &dto.GetPresignedURLsResponse{
		UploadID: req.UploadID,
		URLs:     urls,
	}, nil
}

// CompleteChunkUpload - Dùng Redis để track chunk state, không ghi DB mỗi chunk
// Redis key: upload:{uploadID}:chunks (Hash) - field = chunk_number, value = "etag|size"
// Chỉ flush vào DB khi CompleteVideoUpload được gọi → giảm DB writes từ 205 → 0 trong quá trình upload
func (s *VideoUploadService) CompleteChunkUpload(ctx context.Context, req *dto.CompleteChunkUploadRequest) (*dto.CompleteChunkUploadResponse, error) {
	// Lấy total_chunks từ Redis meta (lưu lúc init), fallback DB nếu Redis không có
	totalChunks := 0
	if s.redisClient != nil {
		redisMetaKey := fmt.Sprintf("upload:%s:meta", req.UploadID)
		if val, err := s.redisClient.HGet(ctx, redisMetaKey, "total_chunks").Result(); err == nil {
			totalChunks, _ = strconv.Atoi(val)
		}
	}
	if totalChunks == 0 {
		// Fallback to DB (Redis miss hoặc không khả dụng)
		upload, err := s.uploadRepo.GetUploadByID(ctx, req.UploadID)
		if err != nil {
			return nil, fmt.Errorf("upload not found: %w", err)
		}
		totalChunks = upload.TotalChunks
	}

	// Validate chunk number
	if req.ChunkNumber < 1 || req.ChunkNumber > totalChunks {
		return nil, fmt.Errorf("invalid chunk number %d", req.ChunkNumber)
	}

	uploadedChunks := 0

	if s.redisClient == nil {
		return nil, fmt.Errorf("redis is required for chunk tracking")
	}

	redisChunksKey := fmt.Sprintf("upload:%s:chunks", req.UploadID)
	chunkValue := fmt.Sprintf("%s|%d", req.ETag, req.Size)

	pipe := s.redisClient.Pipeline()
	pipe.HSet(ctx, redisChunksKey, strconv.Itoa(req.ChunkNumber), chunkValue)
	pipe.Expire(ctx, redisChunksKey, 7*24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to track chunk in redis, please retry: %w", err)
	}

	if count, err := s.redisClient.HLen(ctx, redisChunksKey).Result(); err == nil {
		uploadedChunks = int(count)
	}

	progress := float64(uploadedChunks) / float64(totalChunks) * 100
	isCompleted := uploadedChunks >= totalChunks

	return &dto.CompleteChunkUploadResponse{
		Success:        true,
		ChunkNumber:    req.ChunkNumber,
		Progress:       progress,
		UploadedChunks: uploadedChunks,
		TotalChunks:    totalChunks,
		IsCompleted:    isCompleted,
	}, nil
}

func (s *VideoUploadService) CompleteVideoUpload(ctx context.Context, req *dto.CompleteVideoUploadRequest) (*dto.CompleteVideoUploadResponse, error) {
	// Get upload record with chunks
	upload, err := s.uploadRepo.GetUploadByID(ctx, req.UploadID)
	if err != nil {
		return nil, fmt.Errorf("upload not found: %w", err)
	}

	// Build parts list - đọc từ Redis trước (không cần query DB), fallback DB nếu Redis miss
	var parts []storage.CompletePart
	redisChunksKey := fmt.Sprintf("upload:%s:chunks", req.UploadID)

	if s.redisClient != nil {
		chunkData, redisErr := s.redisClient.HGetAll(ctx, redisChunksKey).Result()
		if redisErr == nil && len(chunkData) > 0 {
			// Đọc ETags từ Redis Hash
			for field, value := range chunkData {
				chunkNum, _ := strconv.Atoi(field)
				valueParts := strings.SplitN(value, "|", 2)
				etag := valueParts[0]
				parts = append(parts, storage.CompletePart{
					PartNumber: chunkNum,
					ETag:       etag,
				})
			}
			// S3 yêu cầu parts phải theo thứ tự
			sort.Slice(parts, func(i, j int) bool {
				return parts[i].PartNumber < parts[j].PartNumber
			})
			if len(parts) < upload.TotalChunks {
				return nil, fmt.Errorf("not all chunks uploaded: %d/%d", len(parts), upload.TotalChunks)
			}

			// 🔄 BULK INSERT Redis → DB trong 1 lần (thay vì 82 UPDATE riêng lẻ)
			dbParts := make([]model.VideoUploadPart, 0, len(chunkData))
			for field, value := range chunkData {
				chunkNum, _ := strconv.Atoi(field)
				valueParts := strings.SplitN(value, "|", 2)
				etag := valueParts[0]
				dbParts = append(dbParts, model.VideoUploadPart{
					UploadID:   upload.ID,
					PartNumber: chunkNum,
					ETag:       etag,
				})
			}
			if dbErr := s.uploadRepo.BulkInsertParts(ctx, dbParts); dbErr != nil {
				log.Printf("⚠️ Failed to bulk insert parts to DB: %v", dbErr)
			}
			progress := float64(len(dbParts)) / float64(upload.TotalChunks) * 100
			if dbErr := s.uploadRepo.SetUploadedChunks(ctx, upload.ID, len(dbParts), progress); dbErr != nil {
				log.Printf("⚠️ Failed to update uploaded chunks count: %v", dbErr)
			}
			log.Printf("✅ Bulk inserted %d parts from Redis to DB for upload %s", len(dbParts), upload.ID)
		}
	}

	if len(parts) == 0 {
		// Fallback: Redis không khả dụng — đọc từ video_upload_parts (chỉ có data nếu đã từng complete trước)
		// Trade-off đã biết: nếu Redis mất data khi upload đang dở → không resume được, user phải upload lại
		dbParts, err := s.uploadRepo.GetPartsByUploadID(ctx, req.UploadID)
		if err != nil {
			return nil, fmt.Errorf("failed to get parts: %w", err)
		}
		if len(dbParts) < upload.TotalChunks {
			return nil, fmt.Errorf("redis unavailable and no part records found (%d/%d) — upload must be restarted", len(dbParts), upload.TotalChunks)
		}
		for _, p := range dbParts {
			parts = append(parts, storage.CompletePart{
				PartNumber: p.PartNumber,
				ETag:       p.ETag,
			})
		}
	}

	// Complete multipart upload in MinIO
	etag, err := s.storage.CompleteMultipartUpload(ctx, upload.Bucket, upload.ObjectKey, upload.S3UploadKey, parts)
	if err != nil {
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	// Update upload status to uploaded
	err = s.uploadRepo.UpdateUploadStatus(ctx, req.UploadID, model.VideoUploadStatusUploaded, nil)
	if err != nil {
		log.Printf("Failed to update upload status to uploaded: %v", err)
		// Don't return error, upload is successful
	}

	// Enqueue processing task
	err = s.enqueueProcessingTask(ctx, upload, etag)
	if err != nil {
		log.Printf("Failed to enqueue processing task: %v", err)
		// Update status to indicate processing queue failed but upload succeeded
		errorMsg := fmt.Sprintf("Upload successful but failed to queue processing: %v", err)
		if updateErr := s.uploadRepo.UpdateUploadStatus(ctx, req.UploadID, model.VideoUploadStatusFailed, &errorMsg); updateErr != nil {
			log.Printf("Failed to update upload status after queue error: %v", updateErr)
		}

		return &dto.CompleteVideoUploadResponse{
			Success:   false,
			UploadID:  req.UploadID,
			Status:    model.VideoUploadStatusFailed,
			ObjectKey: upload.ObjectKey,
			Message:   "Upload completed but processing queue failed. Please contact support.",
		}, nil
	}

	// Update status to processing
	err = s.uploadRepo.UpdateUploadStatus(ctx, req.UploadID, model.VideoUploadStatusProcessing, nil)
	if err != nil {
		log.Printf("Failed to update upload status to processing: %v", err)
	}

	// 🧽 Cleanup Redis: xóa active_uploads, chunks hash, meta hash
	if s.redisClient != nil {
		s.redisClient.SRem(ctx, "active_uploads", req.UploadID.String())
		s.redisClient.Del(ctx,
			fmt.Sprintf("upload:%s:chunks", req.UploadID),
			fmt.Sprintf("upload:%s:meta", req.UploadID),
		)
		log.Printf("✅ Cleaned up Redis keys for upload %s", req.UploadID)
	}

	return &dto.CompleteVideoUploadResponse{
		Success:   true,
		UploadID:  req.UploadID,
		Status:    model.VideoUploadStatusProcessing,
		ObjectKey: upload.ObjectKey,
		Message:   "Upload completed successfully. Processing started.",
	}, nil
}

// GetUploadStatus returns the current status of an upload
func (s *VideoUploadService) GetUploadStatus(ctx context.Context, uploadID uuid.UUID) (*dto.GetUploadStatusResponse, error) {
	upload, err := s.uploadRepo.GetUploadByID(ctx, uploadID)
	if err != nil {
		return nil, fmt.Errorf("upload not found: %w", err)
	}

	// Nếu upload đang active, đọc progress chính xác từ Redis (vì ta không write DB mỗi chunk)
	uploadedChunks := upload.UploadedChunks
	progress := upload.Progress
	if s.redisClient != nil && upload.Status == model.VideoUploadStatusUploading {
		redisChunksKey := fmt.Sprintf("upload:%s:chunks", uploadID)
		if count, err := s.redisClient.HLen(ctx, redisChunksKey).Result(); err == nil && count > 0 {
			uploadedChunks = int(count)
			if upload.TotalChunks > 0 {
				progress = float64(uploadedChunks) / float64(upload.TotalChunks) * 100
			}
		}
	}

	response := &dto.GetUploadStatusResponse{
		UploadID:       upload.ID,
		Status:         upload.Status,
		Progress:       progress,
		UploadedChunks: uploadedChunks,
		TotalChunks:    upload.TotalChunks,
		FileSize:       upload.FileSize,
		RetryCount:     upload.RetryCount,
		CreatedAt:      upload.CreatedAt.Unix(),
		UpdatedAt:      upload.UpdatedAt.Unix(),
	}

	// Add processing info if available
	if upload.ProcessingStartedAt != nil {
		timestamp := upload.ProcessingStartedAt.Unix()
		response.ProcessingStartedAt = &timestamp
	}
	if upload.ProcessingCompletedAt != nil {
		timestamp := upload.ProcessingCompletedAt.Unix()
		response.ProcessingCompletedAt = &timestamp
	}
	if upload.ThumbnailURL != nil {
		response.ThumbnailURL = upload.ThumbnailURL
	}
	if upload.Duration != nil {
		response.Duration = upload.Duration
	}
	if upload.Resolution != nil {
		response.Resolution = upload.Resolution
	}
	if upload.ErrorMessage != nil {
		response.ErrorMessage = upload.ErrorMessage
	}

	return response, nil
}

// GetResumeInfo provides information needed to resume an upload
func (s *VideoUploadService) GetResumeInfo(ctx context.Context, uploadID uuid.UUID) (*dto.GetResumeInfoResponse, error) {
	upload, err := s.uploadRepo.GetUploadByID(ctx, uploadID)
	if err != nil {
		return nil, fmt.Errorf("upload not found: %w", err)
	}

	if !upload.CanResume() {
		return &dto.GetResumeInfoResponse{
			UploadID:  uploadID,
			CanResume: false,
		}, nil
	}

	// Get completed and pending chunks - kiểm tra Redis trước (source of truth khi đang upload)
	var completedChunks []int
	var pendingChunks []int
	redisChunksKey := fmt.Sprintf("upload:%s:chunks", uploadID)

	redisHit := false
	if s.redisClient != nil {
		chunkData, redisErr := s.redisClient.HGetAll(ctx, redisChunksKey).Result()
		if redisErr == nil && len(chunkData) > 0 {
			completedSet := make(map[int]bool, len(chunkData))
			for field := range chunkData {
				n, _ := strconv.Atoi(field)
				completedSet[n] = true
				completedChunks = append(completedChunks, n)
			}
			for i := 1; i <= upload.TotalChunks; i++ {
				if !completedSet[i] {
					pendingChunks = append(pendingChunks, i)
				}
			}
			sort.Ints(completedChunks)
			redisHit = true
		}
	}

	if !redisHit {
		// Fallback: Redis không có data → không biết chunk nào đã xong
		// video_upload_parts chỉ có rows sau khi complete, không có trong quá trình upload
		// → Trả về tất cả là pending (user upload lại từ đầu — trade-off đã chấp nhận)
		log.Printf("⚠️ GetResumeInfo: Redis miss for upload %s, all chunks treated as pending", uploadID)
		for i := 1; i <= upload.TotalChunks; i++ {
			pendingChunks = append(pendingChunks, i)
		}
	}

	progress := float64(len(completedChunks)) / float64(upload.TotalChunks) * 100

	return &dto.GetResumeInfoResponse{
		UploadID:        uploadID,
		MediaID:         upload.ResourceID,
		ObjectKey:       upload.ObjectKey,
		FileName:        upload.OriginalFileName,
		FileSize:        upload.FileSize,
		CanResume:       true,
		CompletedChunks: completedChunks,
		PendingChunks:   pendingChunks,
		ChunkSize:       upload.ChunkSize,
		TotalChunks:     upload.TotalChunks,
		Progress:        progress,
	}, nil
}

// AbortUpload cancels an ongoing upload
func (s *VideoUploadService) AbortUpload(ctx context.Context, req *dto.AbortUploadRequest, uploadID uuid.UUID) (*dto.AbortUploadResponse, error) {
	upload, err := s.uploadRepo.GetUploadByID(ctx, uploadID)
	if err != nil {
		return nil, fmt.Errorf("upload not found: %w", err)
	}

	// Abort multipart upload in MinIO
	if err := s.storage.AbortMultipartUpload(ctx, upload.Bucket, upload.ObjectKey, upload.S3UploadKey); err != nil {
		log.Printf("Failed to abort multipart upload in MinIO: %v", err)
		// Continue with DB cleanup even if MinIO abort fails
	}

	// Update status in DB
	reason := req.Reason
	if reason == "" {
		reason = "Upload aborted by user"
	}

	err = s.uploadRepo.UpdateUploadStatus(ctx, uploadID, model.VideoUploadStatusAborted, &reason)
	if err != nil {
		return nil, fmt.Errorf("failed to update upload status: %w", err)
	}

	return &dto.AbortUploadResponse{
		Success:  true,
		UploadID: uploadID,
		Message:  "Upload aborted successfully",
	}, nil
}

// CleanupExpiredUploads removes expired uploads
func (s *VideoUploadService) CleanupExpiredUploads(ctx context.Context) error {
	uploads, err := s.uploadRepo.GetExpiredUploads(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("failed to get expired uploads: %w", err)
	}

	for _, upload := range uploads {
		// Abort multipart upload in MinIO
		if err := s.storage.AbortMultipartUpload(ctx, upload.Bucket, upload.ObjectKey, upload.S3UploadKey); err != nil {
			log.Printf("Failed to abort expired multipart upload %s: %v", upload.ID, err)
		}

		// Update status
		reason := "Upload expired"
		if err := s.uploadRepo.UpdateUploadStatus(ctx, upload.ID, model.VideoUploadStatusAborted, &reason); err != nil {
			log.Printf("Failed to update expired upload status %s: %v", upload.ID, err)
		}
	}

	return nil
}

// CleanupIncompleteUploads removes old incomplete uploads
func (s *VideoUploadService) CleanupIncompleteUploads(ctx context.Context, olderThanHours int) error {
	cutoff := time.Now().Add(-time.Duration(olderThanHours) * time.Hour)
	uploads, err := s.uploadRepo.GetIncompleteUploadsOlderThan(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("failed to get incomplete uploads: %w", err)
	}

	for _, upload := range uploads {
		// Abort and cleanup similar to expired uploads
		if err := s.storage.AbortMultipartUpload(ctx, upload.Bucket, upload.ObjectKey, upload.S3UploadKey); err != nil {
			log.Printf("Failed to abort incomplete multipart upload %s: %v", upload.ID, err)
		}

		reason := fmt.Sprintf("Incomplete upload cleaned up after %d hours", olderThanHours)
		if err := s.uploadRepo.UpdateUploadStatus(ctx, upload.ID, model.VideoUploadStatusAborted, &reason); err != nil {
			log.Printf("Failed to update incomplete upload status %s: %v", upload.ID, err)
		}
	}

	return nil
}

// Helper methods

func (s *VideoUploadService) validateInitRequest(req *dto.InitVideoUploadRequest) error {
	if req.FileSize <= 0 {
		return fmt.Errorf("invalid file size: %d", req.FileSize)
	}

	// Max file size check (e.g., 5GB)
	maxFileSize := int64(5 * 1024 * 1024 * 1024) // 5GB
	if req.FileSize > maxFileSize {
		return fmt.Errorf("file too large: %d bytes (max: %d bytes)", req.FileSize, maxFileSize)
	}

	// Validate content type
	if !s.storage.ValidateContentType(req.ContentType) {
		return fmt.Errorf("unsupported content type: %s", req.ContentType)
	}

	// Validate chunk size
	if req.ChunkSize > 0 {
		minChunkSize := int64(5 * 1024 * 1024)   // 5MB minimum
		maxChunkSize := int64(100 * 1024 * 1024) // 100MB maximum
		if req.ChunkSize < minChunkSize || req.ChunkSize > maxChunkSize {
			return fmt.Errorf("invalid chunk size: %d (must be between %d and %d)", req.ChunkSize, minChunkSize, maxChunkSize)
		}
	}

	return nil
}

func (s *VideoUploadService) generateObjectKey(resourceType, resourceID, originalFileName string) string {
	// Use storage service to generate consistent object keys
	// if minioStorage, ok := s.storage.(*storage.MinioStorage); ok {
	// 	return minioStorage.GenerateObjectKey(resourceType, resourceID, originalFileName)
	// }

	// Default object key generation
	timestamp := time.Now().Unix()
	return fmt.Sprintf("videos/%s/%s/%d_%s", resourceType, resourceID, timestamp, originalFileName)
}

func (s *VideoUploadService) calculateChunkSize(totalSize, chunkSize int64, chunkNumber, totalChunks int) int64 {
	// For the last chunk, it might be smaller
	if chunkNumber == totalChunks {
		return totalSize - (int64(chunkNumber-1) * chunkSize)
	}
	return chunkSize
}

func (s *VideoUploadService) enqueueProcessingTask(ctx context.Context, upload *model.VideoUpload, etag string) error {
	// Create processing message with default processing options
	message := rabbitmq_queue.VideoProcessingMessage{
		UploadID:     upload.ID,
		ObjectKey:    upload.ObjectKey,
		Bucket:       upload.Bucket,
		UserID:       upload.UserID,
		ResourceID:   upload.ResourceID,
		ResourceType: upload.ResourceType,
		ContentType:  upload.ContentType,
		FileSize:     upload.FileSize,
		ETag:         etag,
		RetryCount:   0,
		CreatedAt:    time.Now().Unix(),
		ProcessingOptions: rabbitmq_queue.VideoProcessingOptions{
			GenerateThumbnail: true,
			ScanForViruses:    false, // Can be enabled based on requirements
			ExtractMetadata:   true,
			GeneratePreview:   false,
			EncodeProfiles:    []string{"480p", "720p", "1080p"}, // HLS multi-resolution
		},
	}

	// Use VideoQueueSetup for proper video processing queue
	if s.videoQueue != nil {
		return s.videoQueue.PublishVideoProcessingMessage(ctx, message)
	}

	// Fallback to generic queue service if VideoQueueSetup not available
	return s.queueService.PublishMessage(ctx, "video", "video.process", message)
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// GetIncompleteUploads retrieves all incomplete uploads for a user
func (s *VideoUploadService) GetIncompleteUploads(ctx context.Context, userID uuid.UUID) ([]model.VideoUpload, error) {
	uploads, err := s.uploadRepo.FindIncompleteByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to find incomplete uploads: %w", err)
	}
	return uploads, nil
}

// GetProcessingQueue retrieves all videos currently in processing for a user
func (s *VideoUploadService) GetProcessingQueue(ctx context.Context, userID uuid.UUID) ([]model.VideoUpload, error) {
	// Find videos that are completed but still processing
	uploads, err := s.uploadRepo.FindByUserIDAndStatus(ctx, userID, model.VideoUploadStatusProcessing)
	if err != nil {
		return nil, fmt.Errorf("failed to find processing uploads: %w", err)
	}
	return uploads, nil
}
