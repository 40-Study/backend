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
	CreateContent(ctx context.Context, lessonID uuid.UUID, userID uuid.UUID, req dto.CreateLessonContentDTO) (*dto.LessonContentResponseDTO, error)
	GetContentByID(ctx context.Context, contentID uuid.UUID) (*dto.LessonContentResponseDTO, error)
	UpdateContent(ctx context.Context, contentID uuid.UUID, req dto.UpdateLessonContentDTO) (*dto.LessonContentResponseDTO, error)
	DeleteContent(ctx context.Context, contentID uuid.UUID) error
	GetContentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]dto.LessonContentResponseDTO, error)
	ReorderContents(ctx context.Context, lessonID uuid.UUID, req dto.ReorderDTO) error
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

func (s *LessonContentService) CreateContent(ctx context.Context, lessonID uuid.UUID, userID uuid.UUID, req dto.CreateLessonContentDTO) (*dto.LessonContentResponseDTO, error) {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	content := &model.LessonContent{
		ID:           uuid.New(),
		LessonID:     lessonID,
		Type:         req.Type,
		Title:        req.Title,
		VideoURL:     req.VideoURL,
		Duration:     0,
		ExerciseID:   req.ExerciseID,
		IsMandatory:  true,
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

func (s *LessonContentService) ReorderContents(ctx context.Context, lessonID uuid.UUID, req dto.ReorderDTO) error {
	if err := s.validateLesson(ctx, lessonID); err != nil {
		return err
	}

	ids := make([]uuid.UUID, len(req.Items))
	items := make([]repository.ReorderItem, len(req.Items))
	for i, item := range req.Items {
		ids[i] = item.ID
		items[i] = repository.ReorderItem{
			ID:           item.ID,
			DisplayOrder: item.DisplayOrder,
		}
	}

	count, err := s.lessonRepo.CountContentsByIDsAndLesson(ctx, ids, lessonID)
	if err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return errors.New("one or more contents do not belong to this lesson")
	}

	return s.lessonRepo.ReorderContents(ctx, items)
}

func (s *LessonContentService) toContentResponseDTO(c *model.LessonContent) *dto.LessonContentResponseDTO {
	return &dto.LessonContentResponseDTO{
		ID:           c.ID,
		LessonID:     c.LessonID,
		Type:         c.Type,
		Title:        c.Title,
		VideoURL:     c.VideoURL,
		Duration:     c.Duration,
		ExerciseID:   c.ExerciseID,
		IsMandatory:  c.IsMandatory,
		DisplayOrder: c.DisplayOrder,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}
