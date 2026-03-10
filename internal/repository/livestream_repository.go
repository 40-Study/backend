package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type LivestreamRepositoryInterface interface {
	Create(ctx context.Context, session *model.LivestreamSession) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error)
	GetByRoomName(ctx context.Context, roomName string) (*model.LivestreamSession, error)
	GetAll(ctx context.Context, page, pageSize int, status string, hostID *uuid.UUID) ([]model.LivestreamSession, int64, error)
	Update(ctx context.Context, session *model.LivestreamSession) error
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.LivestreamSessionStatus) error
	StartSession(ctx context.Context, id uuid.UUID) error
	EndSession(ctx context.Context, id uuid.UUID) error
}

type LivestreamRepository struct {
	db *gorm.DB
}

func NewLivestreamRepository(db *gorm.DB) *LivestreamRepository {
	return &LivestreamRepository{db: db}
}

func (r *LivestreamRepository) Create(ctx context.Context, session *model.LivestreamSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *LivestreamRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	var session model.LivestreamSession
	err := r.db.WithContext(ctx).Preload("Participants").First(&session, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (r *LivestreamRepository) GetByRoomName(ctx context.Context, roomName string) (*model.LivestreamSession, error) {
	var session model.LivestreamSession
	err := r.db.WithContext(ctx).Where("room_name = ?", roomName).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (r *LivestreamRepository) GetAll(ctx context.Context, page, pageSize int, status string, hostID *uuid.UUID) ([]model.LivestreamSession, int64, error) {
	var sessions []model.LivestreamSession
	var total int64

	query := r.db.WithContext(ctx).Model(&model.LivestreamSession{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if hostID != nil {
		query = query.Where("host_id = ?", hostID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := utils.ApplyPagination(query, page, pageSize).
		Order("created_at DESC").
		Find(&sessions).Error

	return sessions, total, err
}

func (r *LivestreamRepository) Update(ctx context.Context, session *model.LivestreamSession) error {
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *LivestreamRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.LivestreamSession{}, "id = ?", id).Error
}

func (r *LivestreamRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status model.LivestreamSessionStatus) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamSession{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (r *LivestreamRepository) StartSession(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamSession{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     model.LivestreamStatusLive,
			"started_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}

func (r *LivestreamRepository) EndSession(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.LivestreamSession{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":   model.LivestreamStatusEnded,
			"ended_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}
