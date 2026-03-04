package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type AttendanceRepositoryInterface interface {
	Create(ctx context.Context, attendance *model.Attendance) error
	GetAll(ctx context.Context, classID uuid.UUID, date *time.Time, page, pageSize int) ([]model.Attendance, int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Attendance, error)
	Update(ctx context.Context, attendance *model.Attendance) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type AttendanceRepository struct {
	db *gorm.DB
}

func NewAttendanceRepository(db *gorm.DB) *AttendanceRepository {
	return &AttendanceRepository{db: db}
}

func (r *AttendanceRepository) Create(ctx context.Context, attendance *model.Attendance) error {
	return r.db.WithContext(ctx).Create(attendance).Error
}

func (r *AttendanceRepository) GetAll(ctx context.Context, classID uuid.UUID, date *time.Time, page, pageSize int) ([]model.Attendance, int64, error) {
	var attendances []model.Attendance
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Attendance{}).Where("class_id = ?", classID)
	if date != nil {
		query = query.Where("date = ?", *date)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Order("date DESC, student_id ASC").
		Find(&attendances).Error; err != nil {
		return nil, 0, err
	}

	return attendances, total, nil
}

func (r *AttendanceRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Attendance, error) {
	var attendance model.Attendance
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&attendance).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &attendance, nil
}

func (r *AttendanceRepository) Update(ctx context.Context, attendance *model.Attendance) error {
	return r.db.WithContext(ctx).Save(attendance).Error
}

func (r *AttendanceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Attendance{}, "id = ?", id).Error
}
