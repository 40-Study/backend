package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type ReportRepositoryInterface interface {
	CreateReport(ctx context.Context, report *model.Report) error
	GetReportByID(ctx context.Context, id uuid.UUID) (*model.Report, error)
	ListReports(ctx context.Context, status, reportedType string, page, pageSize int) ([]model.Report, int64, error)
	UpdateReport(ctx context.Context, report *model.Report) error
	DeleteReport(ctx context.Context, id uuid.UUID) error
	GetReportsByReporter(ctx context.Context, reporterID uuid.UUID, page, pageSize int) ([]model.Report, int64, error)
	CheckDuplicateReport(ctx context.Context, reporterID uuid.UUID, reportedType string, reportedID uuid.UUID) (bool, error)
}

type ReportRepository struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) *ReportRepository {
	return &ReportRepository{db: db}
}

func (r *ReportRepository) CreateReport(ctx context.Context, report *model.Report) error {
	return r.db.WithContext(ctx).Create(report).Error
}

func (r *ReportRepository) GetReportByID(ctx context.Context, id uuid.UUID) (*model.Report, error) {
	var report model.Report
	err := r.db.WithContext(ctx).
		Preload("Reporter").
		Preload("Resolver").
		First(&report, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &report, nil
}

func (r *ReportRepository) ListReports(ctx context.Context, status, reportedType string, page, pageSize int) ([]model.Report, int64, error) {
	var reports []model.Report
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Report{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if reportedType != "" {
		query = query.Where("reported_type = ?", reportedType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.
		Preload("Reporter").
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&reports).Error
	return reports, total, err
}

func (r *ReportRepository) UpdateReport(ctx context.Context, report *model.Report) error {
	return r.db.WithContext(ctx).Save(report).Error
}

func (r *ReportRepository) DeleteReport(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Report{}, "id = ?", id).Error
}

func (r *ReportRepository) GetReportsByReporter(ctx context.Context, reporterID uuid.UUID, page, pageSize int) ([]model.Report, int64, error) {
	var reports []model.Report
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Report{}).Where("reporter_id = ?", reporterID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&reports).Error
	return reports, total, err
}

func (r *ReportRepository) CheckDuplicateReport(ctx context.Context, reporterID uuid.UUID, reportedType string, reportedID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Report{}).
		Where("reporter_id = ? AND reported_type = ? AND reported_id = ? AND status NOT IN ('resolved', 'dismissed')", reporterID, reportedType, reportedID).
		Count(&count).Error
	return count > 0, err
}
