package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type LessonContentServiceInterface interface {
	// Content CRUD
	CreateContent(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonContentDTO) (*dto.LessonContentResponseDTO, error)
	GetContentByID(ctx context.Context, contentID uuid.UUID) (*dto.LessonContentResponseDTO, error)
	UpdateContent(ctx context.Context, contentID uuid.UUID, req dto.UpdateLessonContentDTO) (*dto.LessonContentResponseDTO, error)
	DeleteContent(ctx context.Context, contentID uuid.UUID) error
	GetContentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]dto.LessonContentResponseDTO, error)

	// Session CRUD
	CreateSession(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonSessionDTO) (*dto.LessonSessionResponseDTO, error)
	GetSessionsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]dto.LessonSessionResponseDTO, error)
	UpdateSession(ctx context.Context, sessionID uuid.UUID, req dto.UpdateLessonSessionDTO) (*dto.LessonSessionResponseDTO, error)
	DeleteSession(ctx context.Context, sessionID uuid.UUID) error
}

type LessonContentService struct {
	lessonRepo repository.LessonRepositoryInterface
}

func NewLessonContentService(
	lessonRepo repository.LessonRepositoryInterface,
) *LessonContentService {
	return &LessonContentService{
		lessonRepo: lessonRepo,
	}
}

func (s *LessonContentService) validateLesson(ctx context.Context, lessonID uuid.UUID) error {
	exists, err := s.lessonRepo.Exists(ctx, lessonID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("lesson not found")
	}
	return nil
}

// Content methods

func (s *LessonContentService) CreateContent(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonContentDTO) (*dto.LessonContentResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	content := &model.LessonContent{
		ID:          uuid.New(),
		LessonID:    lessonID,
		Type:        req.Type,
		Title:       req.Title,
		VideoURL:    req.VideoURL,
		Duration:    0,
		StreamID:    req.StreamID,
		ExerciseID:  req.ExerciseID,
		IsMandatory: true,
		DisplayOrder: 0,
	}

	if req.Duration != nil {
		content.Duration = *req.Duration
	}
	if req.IsMandatory != nil {
		content.IsMandatory = *req.IsMandatory
	}
	if req.DisplayOrder != nil {
		content.DisplayOrder = *req.DisplayOrder
	}

	if err := s.lessonRepo.CreateContent(ctx, content); err != nil {
		return nil, err
	}

	return s.toContentResponseDTO(content), nil
}

func (s *LessonContentService) GetContentByID(ctx context.Context, contentID uuid.UUID) (*dto.LessonContentResponseDTO, error) {
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	return s.toContentResponseDTO(content), nil
}

func (s *LessonContentService) GetContentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]dto.LessonContentResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	contents, err := s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.LessonContentResponseDTO, len(contents))
	for i, c := range contents {
		result[i] = *s.toContentResponseDTO(&c)
	}

	return result, nil
}

func (s *LessonContentService) UpdateContent(ctx context.Context, contentID uuid.UUID, req dto.UpdateLessonContentDTO) (*dto.LessonContentResponseDTO, error) {
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	if req.Type != nil {
		content.Type = *req.Type
	}
	if req.Title != nil {
		content.Title = req.Title
	}
	if req.VideoURL != nil {
		content.VideoURL = req.VideoURL
	}
	if req.Duration != nil {
		content.Duration = *req.Duration
	}
	if req.StreamID != nil {
		content.StreamID = req.StreamID
	}
	if req.ExerciseID != nil {
		content.ExerciseID = req.ExerciseID
	}
	if req.IsMandatory != nil {
		content.IsMandatory = *req.IsMandatory
	}
	if req.DisplayOrder != nil {
		content.DisplayOrder = *req.DisplayOrder
	}

	if err := s.lessonRepo.UpdateContent(ctx, content); err != nil {
		return nil, err
	}

	return s.toContentResponseDTO(content), nil
}

func (s *LessonContentService) DeleteContent(ctx context.Context, contentID uuid.UUID) error {
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return err
	}
	if content == nil {
		return errors.New("content not found")
	}

	return s.lessonRepo.DeleteContent(ctx, contentID)
}

// Session methods

func (s *LessonContentService) CreateSession(ctx context.Context, lessonID uuid.UUID, req dto.CreateLessonSessionDTO) (*dto.LessonSessionResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	session := &model.LessonSession{
		ID:          uuid.New(),
		LessonID:    lessonID,
		Title:       req.Title,
		Description: req.Description,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		MeetingURL:  req.MeetingURL,
		MeetingID:   req.MeetingID,
		HostID:      req.HostID,
		Status:      "scheduled",
	}

	if err := s.lessonRepo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	return s.toSessionResponseDTO(session), nil
}

func (s *LessonContentService) GetSessionsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]dto.LessonSessionResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	sessions, err := s.lessonRepo.GetSessionsByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.LessonSessionResponseDTO, len(sessions))
	for i, sess := range sessions {
		result[i] = *s.toSessionResponseDTO(&sess)
	}

	return result, nil
}

func (s *LessonContentService) UpdateSession(ctx context.Context, sessionID uuid.UUID, req dto.UpdateLessonSessionDTO) (*dto.LessonSessionResponseDTO, error) {
	session, err := s.lessonRepo.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}

	if req.Title != nil {
		session.Title = *req.Title
	}
	if req.Description != nil {
		session.Description = req.Description
	}
	if req.StartTime != nil {
		session.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		session.EndTime = req.EndTime
	}
	if req.MeetingURL != nil {
		session.MeetingURL = req.MeetingURL
	}
	if req.MeetingID != nil {
		session.MeetingID = req.MeetingID
	}
	if req.HostID != nil {
		session.HostID = req.HostID
	}
	if req.Status != nil {
		session.Status = *req.Status
	}

	if err := s.lessonRepo.UpdateSession(ctx, session); err != nil {
		return nil, err
	}

	return s.toSessionResponseDTO(session), nil
}

func (s *LessonContentService) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	session, err := s.lessonRepo.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}

	return s.lessonRepo.DeleteSession(ctx, sessionID)
}

// Mappers

func (s *LessonContentService) toContentResponseDTO(c *model.LessonContent) *dto.LessonContentResponseDTO {
	return &dto.LessonContentResponseDTO{
		ID:           c.ID,
		LessonID:     c.LessonID,
		Type:         c.Type,
		Title:        c.Title,
		VideoURL:     c.VideoURL,
		Duration:     c.Duration,
		StreamID:     c.StreamID,
		ExerciseID:   c.ExerciseID,
		IsMandatory:  c.IsMandatory,
		DisplayOrder: c.DisplayOrder,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}

func (s *LessonContentService) toSessionResponseDTO(sess *model.LessonSession) *dto.LessonSessionResponseDTO {
	return &dto.LessonSessionResponseDTO{
		ID:          sess.ID,
		LessonID:    sess.LessonID,
		Title:       sess.Title,
		Description: sess.Description,
		StartTime:   sess.StartTime,
		EndTime:     sess.EndTime,
		MeetingURL:  sess.MeetingURL,
		MeetingID:   sess.MeetingID,
		HostID:      sess.HostID,
		Status:      sess.Status,
		CreatedAt:   sess.CreatedAt,
		UpdatedAt:   sess.UpdatedAt,
	}
}
