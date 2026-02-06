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

type AttendanceServiceInterface interface {
	MarkAttendance(ctx context.Context, classID uuid.UUID, req dto.BulkCreateAttendanceDTO) ([]dto.AttendanceResponseDTO, error)
	GetAll(ctx context.Context, classID uuid.UUID, dateStr string, page, pageSize int) (*dto.AttendanceListResponseDTO, error)
	GetByID(ctx context.Context, id uuid.UUID) (*dto.AttendanceResponseDTO, error)
	Update(ctx context.Context, id uuid.UUID, req dto.UpdateAttendanceDTO) (*dto.AttendanceResponseDTO, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type AttendanceService struct {
	repo repository.AttendanceRepositoryInterface
}

func NewAttendanceService(repo repository.AttendanceRepositoryInterface) *AttendanceService {
	return &AttendanceService{repo: repo}
}

// MarkAttendance records attendance for multiple students in a class for a specific date.
// This is used by teachers to mark student presence/absence.
func (s *AttendanceService) MarkAttendance(ctx context.Context, classID uuid.UUID, req dto.BulkCreateAttendanceDTO) ([]dto.AttendanceResponseDTO, error) {
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return nil, errors.New("invalid date format, expected YYYY-MM-DD")
	}

	result := make([]dto.AttendanceResponseDTO, 0, len(req.Attendances))
	for _, entry := range req.Attendances {
		attendance := &model.Attendance{
			ID:        uuid.New(),
			ClassID:   classID,
			StudentID: entry.StudentID,
			Date:      date,
			Status:    entry.Status,
			Note:      entry.Note,
		}

		if err := s.repo.Create(ctx, attendance); err != nil {
			return nil, err
		}

		result = append(result, *toAttendanceResponseDTO(attendance))
	}

	return result, nil
}

func (s *AttendanceService) GetAll(ctx context.Context, classID uuid.UUID, dateStr string, page, pageSize int) (*dto.AttendanceListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var dateFilter *time.Time
	if dateStr != "" {
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, errors.New("invalid date format, expected YYYY-MM-DD")
		}
		dateFilter = &d
	}

	attendances, total, err := s.repo.GetAll(ctx, classID, dateFilter, page, pageSize)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.AttendanceResponseDTO, len(attendances))
	for i, a := range attendances {
		dtos[i] = *toAttendanceResponseDTO(&a)
	}

	return &dto.AttendanceListResponseDTO{
		Attendances: dtos,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

func (s *AttendanceService) GetByID(ctx context.Context, id uuid.UUID) (*dto.AttendanceResponseDTO, error) {
	attendance, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if attendance == nil {
		return nil, errors.New("attendance not found")
	}
	return toAttendanceResponseDTO(attendance), nil
}

func (s *AttendanceService) Update(ctx context.Context, id uuid.UUID, req dto.UpdateAttendanceDTO) (*dto.AttendanceResponseDTO, error) {
	attendance, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if attendance == nil {
		return nil, errors.New("attendance not found")
	}

	if req.Status != nil {
		attendance.Status = *req.Status
	}
	if req.Note != nil {
		attendance.Note = req.Note
	}

	if err := s.repo.Update(ctx, attendance); err != nil {
		return nil, err
	}

	return toAttendanceResponseDTO(attendance), nil
}

func (s *AttendanceService) Delete(ctx context.Context, id uuid.UUID) error {
	attendance, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if attendance == nil {
		return errors.New("attendance not found")
	}
	return s.repo.Delete(ctx, id)
}

func toAttendanceResponseDTO(a *model.Attendance) *dto.AttendanceResponseDTO {
	return &dto.AttendanceResponseDTO{
		ID:        a.ID,
		ClassID:   a.ClassID,
		StudentID: a.StudentID,
		Date:      a.Date.Format("2006-01-02"),
		Status:    a.Status,
		Note:      a.Note,
		CreatedAt: a.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
