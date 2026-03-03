package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type ClassScheduleRepositoryInterface interface {
	Create(ctx context.Context, schedule *model.ClassSchedule) error
	GetAllByClassID(ctx context.Context, classID uuid.UUID) ([]model.ClassSchedule, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.ClassSchedule, error)
	Update(ctx context.Context, schedule *model.ClassSchedule) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type ClassScheduleRepository struct {
	db *gorm.DB
}

func NewClassScheduleRepository(db *gorm.DB) *ClassScheduleRepository {
	return &ClassScheduleRepository{db: db}
}

func (r *ClassScheduleRepository) Create(ctx context.Context, schedule *model.ClassSchedule) error {
	return r.db.WithContext(ctx).Create(schedule).Error
}

func (r *ClassScheduleRepository) GetAllByClassID(ctx context.Context, classID uuid.UUID) ([]model.ClassSchedule, error) {
	var schedules []model.ClassSchedule
	err := r.db.WithContext(ctx).
		Where("class_id = ?", classID).
		Order("day_of_week ASC, start_time ASC").
		Find(&schedules).Error
	return schedules, err
}

func (r *ClassScheduleRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ClassSchedule, error) {
	var schedule model.ClassSchedule
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&schedule).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &schedule, nil
}

func (r *ClassScheduleRepository) Update(ctx context.Context, schedule *model.ClassSchedule) error {
	return r.db.WithContext(ctx).Save(schedule).Error
}

func (r *ClassScheduleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.ClassSchedule{}, "id = ?", id).Error
}
