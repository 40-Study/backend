package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type WhiteboardRepositoryInterface interface {
	GetSnapshot(ctx context.Context, sessionID uuid.UUID) (*model.WhiteboardSnapshot, error)
	SaveSnapshot(ctx context.Context, snapshot *model.WhiteboardSnapshot) error
	DeleteSnapshot(ctx context.Context, sessionID uuid.UUID) error
}

type WhiteboardRepository struct {
	db *gorm.DB
}

func NewWhiteboardRepository(db *gorm.DB) *WhiteboardRepository {
	return &WhiteboardRepository{db: db}
}

func (r *WhiteboardRepository) GetSnapshot(ctx context.Context, sessionID uuid.UUID) (*model.WhiteboardSnapshot, error) {
	var snapshot model.WhiteboardSnapshot
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&snapshot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &snapshot, nil
}

func (r *WhiteboardRepository) SaveSnapshot(ctx context.Context, snapshot *model.WhiteboardSnapshot) error {
	var existing model.WhiteboardSnapshot
	err := r.db.WithContext(ctx).
		Where("session_id = ?", snapshot.SessionID).
		First(&existing).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.WithContext(ctx).Create(snapshot).Error
		}
		return err
	}

	return r.db.WithContext(ctx).
		Model(&model.WhiteboardSnapshot{}).
		Where("session_id = ?", snapshot.SessionID).
		Updates(map[string]interface{}{
			"snapshot_data": snapshot.SnapshotData,
			"version":       gorm.Expr("version + 1"),
			"saved_at":      gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}

func (r *WhiteboardRepository) DeleteSnapshot(ctx context.Context, sessionID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.WhiteboardSnapshot{}).Error
}

type AnalyticsRepositoryInterface interface {
	GetBySession(ctx context.Context, sessionID uuid.UUID) (*model.LivestreamAnalytics, error)
	Create(ctx context.Context, analytics *model.LivestreamAnalytics) error
	Update(ctx context.Context, analytics *model.LivestreamAnalytics) error
	UpdatePeakViewers(ctx context.Context, sessionID uuid.UUID, peak int) error
	IncrementTotalViewers(ctx context.Context, sessionID uuid.UUID) error
	IncrementTotalMessages(ctx context.Context, sessionID uuid.UUID) error
	GetSubmissionStats(ctx context.Context, assignmentID uuid.UUID) (map[string]interface{}, error)
}

type AnalyticsRepository struct {
	db *gorm.DB
}

func NewAnalyticsRepository(db *gorm.DB) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

func (r *AnalyticsRepository) GetBySession(ctx context.Context, sessionID uuid.UUID) (*model.LivestreamAnalytics, error) {
	var analytics model.LivestreamAnalytics
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&analytics).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &analytics, nil
}

func (r *AnalyticsRepository) Create(ctx context.Context, analytics *model.LivestreamAnalytics) error {
	return r.db.WithContext(ctx).Create(analytics).Error
}

func (r *AnalyticsRepository) Update(ctx context.Context, analytics *model.LivestreamAnalytics) error {
	return r.db.WithContext(ctx).Save(analytics).Error
}

func (r *AnalyticsRepository) UpdatePeakViewers(ctx context.Context, sessionID uuid.UUID, peak int) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamAnalytics{}).
		Where("session_id = ? AND peak_viewers < ?", sessionID, peak).
		Update("peak_viewers", peak).Error
}

func (r *AnalyticsRepository) IncrementTotalViewers(ctx context.Context, sessionID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamAnalytics{}).
		Where("session_id = ?", sessionID).
		UpdateColumn("total_viewers", gorm.Expr("total_viewers + 1")).Error
}

func (r *AnalyticsRepository) IncrementTotalMessages(ctx context.Context, sessionID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamAnalytics{}).
		Where("session_id = ?", sessionID).
		UpdateColumn("total_messages", gorm.Expr("total_messages + 1")).Error
}

func (r *AnalyticsRepository) GetSubmissionStats(ctx context.Context, assignmentID uuid.UUID) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var total, accepted int64
	if err := r.db.WithContext(ctx).
		Model(&model.Submission{}).
		Where("assignment_id = ?", assignmentID).
		Count(&total).Error; err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).
		Model(&model.Submission{}).
		Where("assignment_id = ? AND verdict = ?", assignmentID, model.VerdictAccepted).
		Count(&accepted).Error; err != nil {
		return nil, err
	}

	stats["total_submissions"] = total
	stats["accepted_count"] = accepted
	if total > 0 {
		stats["acceptance_rate"] = float64(accepted) / float64(total) * 100
	} else {
		stats["acceptance_rate"] = 0.0
	}

	return stats, nil
}
