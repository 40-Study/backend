package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type ReportServiceInterface interface {
	CreateReport(ctx context.Context, reporterID uuid.UUID, req dto.CreateReportDTO) (*dto.ReportResponseDTO, error)
	GetReportByID(ctx context.Context, id uuid.UUID) (*dto.ReportResponseDTO, error)
	ListReports(ctx context.Context, status, reportedType string, page, pageSize int) (*dto.ReportListDTO, error)
	UpdateReportStatus(ctx context.Context, id, adminID uuid.UUID, req dto.UpdateReportStatusDTO) (*dto.ReportResponseDTO, error)
	DeleteReport(ctx context.Context, id uuid.UUID) error
	GetMyReports(ctx context.Context, reporterID uuid.UUID, page, pageSize int) (*dto.ReportListDTO, error)
}

type ReportService struct {
	repo repository.ReportRepositoryInterface
}

func NewReportService(repo repository.ReportRepositoryInterface) *ReportService {
	return &ReportService{repo: repo}
}

func (s *ReportService) CreateReport(ctx context.Context, reporterID uuid.UUID, req dto.CreateReportDTO) (*dto.ReportResponseDTO, error) {
	reportedID, err := uuid.Parse(req.ReportedID)
	if err != nil {
		return nil, errors.New("invalid reported_id")
	}

	// Check duplicate
	isDuplicate, _ := s.repo.CheckDuplicateReport(ctx, reporterID, req.ReportedType, reportedID)
	if isDuplicate {
		return nil, errors.New("you have already reported this content")
	}

	report := &model.Report{
		ReporterID:   reporterID,
		ReportedType: req.ReportedType,
		ReportedID:   reportedID,
		Reason:       req.Reason,
		Status:       "pending",
	}
	if req.Description != "" {
		report.Description = &req.Description
	}

	if err := s.repo.CreateReport(ctx, report); err != nil {
		return nil, err
	}

	loaded, _ := s.repo.GetReportByID(ctx, report.ID)
	if loaded != nil {
		return s.mapReportToDTO(loaded), nil
	}
	return s.mapReportToDTO(report), nil
}

func (s *ReportService) GetReportByID(ctx context.Context, id uuid.UUID) (*dto.ReportResponseDTO, error) {
	report, err := s.repo.GetReportByID(ctx, id)
	if err != nil || report == nil {
		return nil, errors.New("report not found")
	}
	return s.mapReportToDTO(report), nil
}

func (s *ReportService) ListReports(ctx context.Context, status, reportedType string, page, pageSize int) (*dto.ReportListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	reports, total, err := s.repo.ListReports(ctx, status, reportedType, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.ReportResponseDTO, len(reports))
	for i, r := range reports {
		data[i] = *s.mapReportToDTO(&r)
	}

	return &dto.ReportListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ReportService) UpdateReportStatus(ctx context.Context, id, adminID uuid.UUID, req dto.UpdateReportStatusDTO) (*dto.ReportResponseDTO, error) {
	report, err := s.repo.GetReportByID(ctx, id)
	if err != nil || report == nil {
		return nil, errors.New("report not found")
	}

	report.Status = req.Status
	if req.AdminNotes != "" {
		report.AdminNotes = &req.AdminNotes
	}

	if req.Status == "resolved" || req.Status == "dismissed" {
		now := time.Now()
		report.ResolvedBy = &adminID
		report.ResolvedAt = &now
	}

	if err := s.repo.UpdateReport(ctx, report); err != nil {
		return nil, err
	}

	return s.mapReportToDTO(report), nil
}

func (s *ReportService) DeleteReport(ctx context.Context, id uuid.UUID) error {
	report, err := s.repo.GetReportByID(ctx, id)
	if err != nil || report == nil {
		return errors.New("report not found")
	}
	return s.repo.DeleteReport(ctx, id)
}

func (s *ReportService) GetMyReports(ctx context.Context, reporterID uuid.UUID, page, pageSize int) (*dto.ReportListDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	reports, total, err := s.repo.GetReportsByReporter(ctx, reporterID, page, pageSize)
	if err != nil {
		return nil, err
	}

	data := make([]dto.ReportResponseDTO, len(reports))
	for i, r := range reports {
		data[i] = *s.mapReportToDTO(&r)
	}

	return &dto.ReportListDTO{Data: data, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ReportService) mapReportToDTO(r *model.Report) *dto.ReportResponseDTO {
	reporterName := ""
	if r.Reporter.FullName != nil {
		reporterName = *r.Reporter.FullName
	} else {
		reporterName = r.Reporter.UserName
	}

	return &dto.ReportResponseDTO{
		ID:           r.ID,
		ReporterID:   r.ReporterID,
		ReporterName: reporterName,
		ReportedType: r.ReportedType,
		ReportedID:   r.ReportedID,
		Reason:       r.Reason,
		Description:  r.Description,
		Status:       r.Status,
		AdminNotes:   r.AdminNotes,
		ResolvedBy:   r.ResolvedBy,
		ResolvedAt:   r.ResolvedAt,
		CreatedAt:    r.CreatedAt,
	}
}
