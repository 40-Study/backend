package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	asynq_queue "study.com/v1/internal/queue/asynq"
	"study.com/v1/internal/repository"
)

type PersonalEventServiceInterface interface {
	CreateEvent(ctx context.Context, userID uuid.UUID, req dto.CreatePersonalEventDTO) (*dto.PersonalEventResponseDTO, error)
	UpdateEvent(ctx context.Context, userID, eventID uuid.UUID, req dto.UpdatePersonalEventDTO) (*dto.PersonalEventResponseDTO, error)
	DeleteEvent(ctx context.Context, userID, eventID uuid.UUID) error
	GetMyEvents(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]dto.PersonalEventResponseDTO, error)
}

type PersonalEventService struct {
	repo  repository.PersonalEventRepositoryInterface
	queue *asynq_queue.Queue
}

func NewPersonalEventService(
	repo repository.PersonalEventRepositoryInterface,
	queue *asynq_queue.Queue,
) *PersonalEventService {
	return &PersonalEventService{
		repo:  repo,
		queue: queue,
	}
}

func (s *PersonalEventService) CreateEvent(ctx context.Context, userID uuid.UUID, req dto.CreatePersonalEventDTO) (*dto.PersonalEventResponseDTO, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.New("invalid start_time format, expected RFC3339")
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.New("invalid end_time format, expected RFC3339")
	}

	if endTime.Before(startTime) {
		return nil, errors.New("end_time must be after start_time")
	}

	event := &model.PersonalEvent{
		UserID:      userID,
		Title:       req.Title,
		Description: req.Description,
		StartTime:   startTime,
		EndTime:     endTime,
		Color:       req.Color,
		Location:    req.Location,
		IsAllDay:    req.IsAllDay,
		ReminderMin: req.ReminderMin,
	}

	if err := s.repo.Create(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to create personal event: %w", err)
	}

	// Schedule reminder if reminder_minutes is set
	s.scheduleReminder(event)

	return s.toResponseDTO(event), nil
}

func (s *PersonalEventService) UpdateEvent(ctx context.Context, userID, eventID uuid.UUID, req dto.UpdatePersonalEventDTO) (*dto.PersonalEventResponseDTO, error) {
	event, err := s.repo.GetByID(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get personal event: %w", err)
	}
	if event == nil {
		return nil, errors.New("event not found")
	}
	if event.UserID != userID {
		return nil, errors.New("you do not own this event")
	}

	if req.Title != nil {
		event.Title = *req.Title
	}
	if req.Description != nil {
		event.Description = req.Description
	}
	if req.StartTime != nil {
		startTime, err := time.Parse(time.RFC3339, *req.StartTime)
		if err != nil {
			return nil, errors.New("invalid start_time format, expected RFC3339")
		}
		event.StartTime = startTime
	}
	if req.EndTime != nil {
		endTime, err := time.Parse(time.RFC3339, *req.EndTime)
		if err != nil {
			return nil, errors.New("invalid end_time format, expected RFC3339")
		}
		event.EndTime = endTime
	}
	if event.EndTime.Before(event.StartTime) {
		return nil, errors.New("end_time must be after start_time")
	}
	if req.Color != nil {
		event.Color = req.Color
	}
	if req.Location != nil {
		event.Location = req.Location
	}
	if req.IsAllDay != nil {
		event.IsAllDay = *req.IsAllDay
	}
	if req.ReminderMin != nil {
		event.ReminderMin = req.ReminderMin
	}

	if err := s.repo.Update(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to update personal event: %w", err)
	}

	// Re-schedule reminder if reminder_minutes changed
	if req.ReminderMin != nil || req.StartTime != nil {
		s.scheduleReminder(event)
	}

	return s.toResponseDTO(event), nil
}

func (s *PersonalEventService) DeleteEvent(ctx context.Context, userID, eventID uuid.UUID) error {
	event, err := s.repo.GetByID(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to get personal event: %w", err)
	}
	if event == nil {
		return errors.New("event not found")
	}
	if event.UserID != userID {
		return errors.New("you do not own this event")
	}

	return s.repo.Delete(ctx, eventID)
}

func (s *PersonalEventService) GetMyEvents(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]dto.PersonalEventResponseDTO, error) {
	events, err := s.repo.ListByUserAndDateRange(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to list personal events: %w", err)
	}

	result := make([]dto.PersonalEventResponseDTO, 0, len(events))
	for i := range events {
		result = append(result, *s.toResponseDTO(&events[i]))
	}
	return result, nil
}

func (s *PersonalEventService) scheduleReminder(event *model.PersonalEvent) {
	if s.queue == nil || event.ReminderMin == nil || *event.ReminderMin <= 0 {
		return
	}

	reminderAt := event.StartTime.Add(-time.Duration(*event.ReminderMin) * time.Minute)
	if reminderAt.Before(time.Now()) {
		return
	}

	payload := asynq_queue.EventReminderPayload{
		EventID: event.ID,
		UserID:  event.UserID,
		Title:   event.Title,
	}

	if err := s.queue.ScheduleAt(asynq_queue.TaskEventReminder, payload, reminderAt, "notifications"); err != nil {
		log.Printf("[personal_event] Failed to schedule reminder for event %s: %v", event.ID, err)
	}
}

func (s *PersonalEventService) toResponseDTO(event *model.PersonalEvent) *dto.PersonalEventResponseDTO {
	return &dto.PersonalEventResponseDTO{
		ID:          event.ID,
		UserID:      event.UserID,
		Title:       event.Title,
		Description: event.Description,
		StartTime:   event.StartTime,
		EndTime:     event.EndTime,
		Color:       event.Color,
		Location:    event.Location,
		IsAllDay:    event.IsAllDay,
		ReminderMin: event.ReminderMin,
		CreatedAt:   event.CreatedAt,
		UpdatedAt:   event.UpdatedAt,
	}
}
