package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// VideoUploadRepositoryInterface - Interface cho repository quản lý video upload trong PostgreSQL
type VideoUploadRepositoryInterface interface {
	// CRUD cho Video Upload
	CreateUpload(ctx context.Context, upload *model.VideoUpload) error
	GetUploadByID(ctx context.Context, uploadID uuid.UUID) (*model.VideoUpload, error)
	GetUploadByUploadKey(ctx context.Context, uploadKey string) (*model.VideoUpload, error)
	FindUploadsByFilename(ctx context.Context, userID uuid.UUID, filename string, fileSize int64) ([]*model.VideoUpload, error)
	UpdateUpload(ctx context.Context, upload *model.VideoUpload) error
	UpdateUploadStatus(ctx context.Context, uploadID uuid.UUID, status model.VideoUploadStatus, errorMsg *string) error
	DeleteUpload(ctx context.Context, uploadID uuid.UUID) error

	// Parts - chỉ INSERT sau khi upload hoàn tất (flush từ Redis)
	BulkInsertParts(ctx context.Context, parts []model.VideoUploadPart) error
	GetPartsByUploadID(ctx context.Context, uploadID uuid.UUID) ([]*model.VideoUploadPart, error)

	// Progress
	SetUploadedChunks(ctx context.Context, uploadID uuid.UUID, count int, progress float64) error

	// Cleanup operations
	GetExpiredUploads(ctx context.Context, before time.Time) ([]*model.VideoUpload, error)
	GetIncompleteUploadsOlderThan(ctx context.Context, olderThan time.Time) ([]*model.VideoUpload, error)

	// Status queries
	GetUploadsByStatus(ctx context.Context, status model.VideoUploadStatus, limit int) ([]*model.VideoUpload, error)
	GetUploadsByUserID(ctx context.Context, userID uuid.UUID, limit int, offset int) ([]*model.VideoUpload, error)
	FindIncompleteByUserID(ctx context.Context, userID uuid.UUID) ([]model.VideoUpload, error)
	FindByUserIDAndStatus(ctx context.Context, userID uuid.UUID, status model.VideoUploadStatus) ([]model.VideoUpload, error)
}

type VideoUploadRepository struct {
	db *gorm.DB
}

func NewVideoUploadRepository(db *gorm.DB) *VideoUploadRepository {
	return &VideoUploadRepository{db: db}
}

// CreateUpload creates a new video upload record
func (r *VideoUploadRepository) CreateUpload(ctx context.Context, upload *model.VideoUpload) error {
	if upload.ID == uuid.Nil {
		upload.ID = uuid.New()
	}

	result := r.db.WithContext(ctx).Create(upload)
	if result.Error != nil {
		return fmt.Errorf("failed to create video upload: %w", result.Error)
	}

	return nil
}

// GetUploadByID retrieves upload by UUID
func (r *VideoUploadRepository) GetUploadByID(ctx context.Context, uploadID uuid.UUID) (*model.VideoUpload, error) {
	var upload model.VideoUpload

	result := r.db.WithContext(ctx).Where("id = ?", uploadID).First(&upload)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("video upload not found: %s", uploadID.String())
		}
		return nil, fmt.Errorf("failed to get video upload: %w", result.Error)
	}

	return &upload, nil
}

// GetUploadByUploadKey retrieves upload by S3 upload key
func (r *VideoUploadRepository) GetUploadByUploadKey(ctx context.Context, uploadKey string) (*model.VideoUpload, error) {
	var upload model.VideoUpload

	result := r.db.WithContext(ctx).Where("upload_id = ?", uploadKey).First(&upload)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("video upload not found for key: %s", uploadKey)
		}
		return nil, fmt.Errorf("failed to get video upload by key: %w", result.Error)
	}

	return &upload, nil
}

// FindUploadsByFilename finds uploads by user, filename and file size
func (r *VideoUploadRepository) FindUploadsByFilename(ctx context.Context, userID uuid.UUID, filename string, fileSize int64) ([]*model.VideoUpload, error) {
	var uploads []*model.VideoUpload

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND original_file_name = ? AND file_size = ?", userID, filename, fileSize).
		Order("created_at DESC").
		Limit(5). // Only check last 5 uploads
		Find(&uploads)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to find uploads by filename: %w", result.Error)
	}

	return uploads, nil
}

// UpdateUpload updates the entire upload record
func (r *VideoUploadRepository) UpdateUpload(ctx context.Context, upload *model.VideoUpload) error {
	result := r.db.WithContext(ctx).Save(upload)
	if result.Error != nil {
		return fmt.Errorf("failed to update video upload: %w", result.Error)
	}

	return nil
}

// UpdateUploadStatus updates only status and error message
func (r *VideoUploadRepository) UpdateUploadStatus(ctx context.Context, uploadID uuid.UUID, status model.VideoUploadStatus, errorMsg *string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}

	if errorMsg != nil {
		updates["error_message"] = *errorMsg
	}

	result := r.db.WithContext(ctx).Model(&model.VideoUpload{}).
		Where("id = ?", uploadID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update upload status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no upload found with ID: %s", uploadID.String())
	}

	return nil
}

// DeleteUpload removes an upload and its associated parts
func (r *VideoUploadRepository) DeleteUpload(ctx context.Context, uploadID uuid.UUID) error {
	// Delete associated parts first
	if err := r.db.WithContext(ctx).Where("upload_id = ?", uploadID).Delete(&model.VideoUploadPart{}).Error; err != nil {
		return fmt.Errorf("failed to delete upload parts: %w", err)
	}
	if err := r.db.WithContext(ctx).Where("id = ?", uploadID).Delete(&model.VideoUpload{}).Error; err != nil {
		return fmt.Errorf("failed to delete video upload: %w", err)
	}
	return nil
}

// BulkInsertParts inserts all parts in a single batch operation (called once at CompleteVideoUpload)
func (r *VideoUploadRepository) BulkInsertParts(ctx context.Context, parts []model.VideoUploadPart) error {
	if len(parts) == 0 {
		return nil
	}
	// CreateInBatches với batch size 100 để tránh PostgreSQL parameter limit
	result := r.db.WithContext(ctx).CreateInBatches(parts, 100)
	if result.Error != nil {
		return fmt.Errorf("failed to bulk insert parts: %w", result.Error)
	}
	return nil
}

// GetPartsByUploadID retrieves all parts for an upload (for audit/verify after completion)
func (r *VideoUploadRepository) GetPartsByUploadID(ctx context.Context, uploadID uuid.UUID) ([]*model.VideoUploadPart, error) {
	var parts []*model.VideoUploadPart
	result := r.db.WithContext(ctx).
		Where("upload_id = ?", uploadID).
		Order("part_number ASC").
		Find(&parts)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get parts: %w", result.Error)
	}
	return parts, nil
}

// SetUploadedChunks updates uploaded_chunks count and progress on video_uploads
func (r *VideoUploadRepository) SetUploadedChunks(ctx context.Context, uploadID uuid.UUID, count int, progress float64) error {
	result := r.db.WithContext(ctx).Model(&model.VideoUpload{}).
		Where("id = ?", uploadID).
		Updates(map[string]interface{}{
			"uploaded_chunks": count,
			"progress":        progress,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to set uploaded chunks: %w", result.Error)
	}
	return nil
}

// GetExpiredUploads returns uploads that have exceeded their expiration time
func (r *VideoUploadRepository) GetExpiredUploads(ctx context.Context, before time.Time) ([]*model.VideoUpload, error) {
	var uploads []*model.VideoUpload

	result := r.db.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at < ?", before).
		Where("status NOT IN ?", []model.VideoUploadStatus{
			model.VideoUploadStatusReady,
			model.VideoUploadStatusFailed,
			model.VideoUploadStatusAborted,
		}).
		Find(&uploads)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get expired uploads: %w", result.Error)
	}

	return uploads, nil
}

// GetIncompleteUploadsOlderThan returns incomplete uploads older than specified time
func (r *VideoUploadRepository) GetIncompleteUploadsOlderThan(ctx context.Context, olderThan time.Time) ([]*model.VideoUpload, error) {
	var uploads []*model.VideoUpload

	result := r.db.WithContext(ctx).
		Where("created_at < ?", olderThan).
		Where("status IN ?", []model.VideoUploadStatus{
			model.VideoUploadStatusInitialized,
			model.VideoUploadStatusUploading,
		}).
		Find(&uploads)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get incomplete uploads: %w", result.Error)
	}

	return uploads, nil
}

// GetUploadsByStatus returns uploads with specific status
func (r *VideoUploadRepository) GetUploadsByStatus(ctx context.Context, status model.VideoUploadStatus, limit int) ([]*model.VideoUpload, error) {
	var uploads []*model.VideoUpload

	query := r.db.WithContext(ctx).Where("status = ?", status)
	if limit > 0 {
		query = query.Limit(limit)
	}

	result := query.Find(&uploads)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get uploads by status: %w", result.Error)
	}

	return uploads, nil
}

// GetUploadsByUserID returns uploads for a specific user with pagination
func (r *VideoUploadRepository) GetUploadsByUserID(ctx context.Context, userID uuid.UUID, limit int, offset int) ([]*model.VideoUpload, error) {
	var uploads []*model.VideoUpload

	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&uploads)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get uploads by user: %w", result.Error)
	}

	return uploads, nil
}

// FindIncompleteByUserID returns incomplete uploads for a specific user
func (r *VideoUploadRepository) FindIncompleteByUserID(ctx context.Context, userID uuid.UUID) ([]model.VideoUpload, error) {
	var uploads []model.VideoUpload

	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("status IN ?", []model.VideoUploadStatus{
			model.VideoUploadStatusInitialized,
			model.VideoUploadStatusUploading,
		}).
		Order("created_at DESC").
		Find(&uploads)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to find incomplete uploads: %w", result.Error)
	}

	return uploads, nil
}

// FindByUserIDAndStatus returns uploads for a specific user with a specific status
func (r *VideoUploadRepository) FindByUserIDAndStatus(ctx context.Context, userID uuid.UUID, status model.VideoUploadStatus) ([]model.VideoUpload, error) {
	var uploads []model.VideoUpload

	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("status = ?", status).
		Order("created_at DESC").
		Find(&uploads)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to find uploads by user and status: %w", result.Error)
	}

	return uploads, nil
}
