package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type ClassScheduleServiceInterface interface {
	Create(ctx context.Context, classID uuid.UUID, req dto.CreateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error)
	GetAllByClassID(ctx context.Context, classID uuid.UUID) ([]dto.ClassScheduleResponseDTO, error)
	Update(ctx context.Context, id uuid.UUID, req dto.UpdateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type ClassScheduleService struct {
	repo      repository.ClassScheduleRepositoryInterface
	classRepo repository.ClassRepositoryInterface
}

func NewClassScheduleService(
	repo repository.ClassScheduleRepositoryInterface,
	classRepo repository.ClassRepositoryInterface,
) *ClassScheduleService {
	return &ClassScheduleService{
		repo:      repo,
		classRepo: classRepo,
	}
}

func (s *ClassScheduleService) Create(ctx context.Context, classID uuid.UUID, req dto.CreateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error) {
	// Check class exists
	exists, err := s.classRepo.Exists(ctx, classID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("class not found")
	}

	schedule := &model.ClassSchedule{
		ID:        uuid.New(),
		ClassID:   classID,
		DayOfWeek: req.DayOfWeek,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Room:      req.Room,
	}

	if err := s.repo.Create(ctx, schedule); err != nil {
		return nil, err
	}

	return toClassScheduleResponseDTO(schedule), nil
}

func (s *ClassScheduleService) GetAllByClassID(ctx context.Context, classID uuid.UUID) ([]dto.ClassScheduleResponseDTO, error) {
	schedules, err := s.repo.GetAllByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.ClassScheduleResponseDTO, len(schedules))
	for i, sch := range schedules {
		result[i] = *toClassScheduleResponseDTO(&sch)
	}

	return result, nil
}

func (s *ClassScheduleService) Update(ctx context.Context, id uuid.UUID, req dto.UpdateClassScheduleDTO) (*dto.ClassScheduleResponseDTO, error) {
	schedule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if schedule == nil {
		return nil, errors.New("schedule not found")
	}

	if req.DayOfWeek != nil {
		schedule.DayOfWeek = *req.DayOfWeek
	}
	if req.StartTime != nil {
		schedule.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		schedule.EndTime = *req.EndTime
	}
	if req.Room != nil {
		schedule.Room = req.Room
	}

	if err := s.repo.Update(ctx, schedule); err != nil {
		return nil, err
	}

	return toClassScheduleResponseDTO(schedule), nil
}

func (s *ClassScheduleService) Delete(ctx context.Context, id uuid.UUID) error {
	schedule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if schedule == nil {
		return errors.New("schedule not found")
	}
	return s.repo.Delete(ctx, id)
}

func toClassScheduleResponseDTO(s *model.ClassSchedule) *dto.ClassScheduleResponseDTO {
	return &dto.ClassScheduleResponseDTO{
		ID:        s.ID,
		ClassID:   s.ClassID,
		DayOfWeek: s.DayOfWeek,
		StartTime: s.StartTime,
		EndTime:   s.EndTime,
		Room:      s.Room,
		CreatedAt: s.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
