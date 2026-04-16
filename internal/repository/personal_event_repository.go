package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type PersonalEventRepositoryInterface interface {
	Create(ctx context.Context, event *model.PersonalEvent) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.PersonalEvent, error)
	Update(ctx context.Context, event *model.PersonalEvent) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByUserAndDateRange(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]model.PersonalEvent, error)
}

type PersonalEventRepository struct {
	db *gorm.DB
}

func NewPersonalEventRepository(db *gorm.DB) *PersonalEventRepository {
	return &PersonalEventRepository{db: db}
}

func (r *PersonalEventRepository) Create(ctx context.Context, event *model.PersonalEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *PersonalEventRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.PersonalEvent, error) {
	var event model.PersonalEvent
	err := r.db.WithContext(ctx).First(&event, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

func (r *PersonalEventRepository) Update(ctx context.Context, event *model.PersonalEvent) error {
	return r.db.WithContext(ctx).Save(event).Error
}

func (r *PersonalEventRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.PersonalEvent{}, "id = ?", id).Error
}

func (r *PersonalEventRepository) ListByUserAndDateRange(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]model.PersonalEvent, error) {
	var events []model.PersonalEvent
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND start_time <= ? AND end_time >= ?", userID, endDate, startDate).
		Order("start_time ASC").
		Find(&events).Error
	return events, err
}
