package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type LessonServiceInterface interface {
	CreateLesson(ctx context.Context, sectionID uuid.UUID, req dto.CreateLessonDTO) (*dto.LessonResponseDTO, error)
	GetAllLessons(ctx context.Context, sectionID uuid.UUID) ([]dto.LessonResponseDTO, error)
	GetLessonByID(ctx context.Context, lessonID uuid.UUID) (*dto.LessonResponseDTO, error)
	UpdateLesson(ctx context.Context, lessonID uuid.UUID, req dto.UpdateLessonDTO) (*dto.LessonResponseDTO, error)
	DeleteLesson(ctx context.Context, lessonID uuid.UUID) error
	ReorderLessons(ctx context.Context, sectionID uuid.UUID, req dto.ReorderDTO) error
}

type LessonService struct {
	lessonRepo     repository.LessonRepositoryInterface
	sectionRepo    repository.SectionRepositoryInterface
}

func NewLessonService(
	lessonRepo repository.LessonRepositoryInterface,
	sectionRepo repository.SectionRepositoryInterface,
) *LessonService {
	return &LessonService{
		lessonRepo:  lessonRepo,
		sectionRepo: sectionRepo,
	}
}

func (s *LessonService) validateSection(ctx context.Context, sectionID uuid.UUID) error {
	exists, err := s.sectionRepo.Exists(ctx, sectionID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("section not found")
	}
	return nil
}

func (s *LessonService) CreateLesson(ctx context.Context, sectionID uuid.UUID, req dto.CreateLessonDTO) (*dto.LessonResponseDTO, error) {
	if err := s.validateSection(ctx, sectionID); err != nil {
		return nil, err
	}

	maxOrder, err := s.lessonRepo.GetMaxDisplayOrder(ctx, sectionID)
	if err != nil {
		return nil, err
	}

	displayOrder := maxOrder + 1
	if req.DisplayOrder > 0 {
		displayOrder = req.DisplayOrder
	}

	lesson := &model.Lesson{
		ID:           uuid.New(),
		SectionID:    sectionID,
		Title:        req.Title,
		Description:  req.Description,
		DisplayOrder: displayOrder,
		IsMandatory:  true,
	}

	if req.DurationMins != nil {
		lesson.DurationMins = *req.DurationMins
	}
	if req.IsPreview != nil {
		lesson.IsPreview = *req.IsPreview
	}
	if req.IsMandatory != nil {
		lesson.IsMandatory = *req.IsMandatory
	}

	if err := s.lessonRepo.Create(ctx, lesson); err != nil {
		return nil, err
	}

	return s.toLessonResponseDTO(lesson, nil), nil
}

func (s *LessonService) GetAllLessons(ctx context.Context, sectionID uuid.UUID) ([]dto.LessonResponseDTO, error) {
	if err := s.validateSection(ctx, sectionID); err != nil {
		return nil, err
	}

	lessons, err := s.lessonRepo.GetAllBySectionID(ctx, sectionID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.LessonResponseDTO, len(lessons))
	for i, les := range lessons {
		result[i] = *s.toLessonResponseDTO(&les, nil)
	}

	return result, nil
}

func (s *LessonService) GetLessonByID(ctx context.Context, lessonID uuid.UUID) (*dto.LessonResponseDTO, error) {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil {
		return nil, errors.New("lesson not found")
	}

	contents, err := s.lessonRepo.GetContentsByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	return s.toLessonResponseDTO(lesson, contents), nil
}

func (s *LessonService) UpdateLesson(ctx context.Context, lessonID uuid.UUID, req dto.UpdateLessonDTO) (*dto.LessonResponseDTO, error) {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil {
		return nil, errors.New("lesson not found")
	}

	if req.Title != nil {
		lesson.Title = *req.Title
	}
	if req.Description != nil {
		lesson.Description = req.Description
	}
	if req.DisplayOrder != nil {
		lesson.DisplayOrder = *req.DisplayOrder
	}
	if req.DurationMins != nil {
		lesson.DurationMins = *req.DurationMins
	}
	if req.IsPreview != nil {
		lesson.IsPreview = *req.IsPreview
	}
	if req.IsMandatory != nil {
		lesson.IsMandatory = *req.IsMandatory
	}

	if err := s.lessonRepo.Update(ctx, lesson); err != nil {
		return nil, err
	}

	contents, _ := s.lessonRepo.GetContentsByLessonID(ctx, lessonID)

	return s.toLessonResponseDTO(lesson, contents), nil
}

func (s *LessonService) DeleteLesson(ctx context.Context, lessonID uuid.UUID) error {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return err
	}
	if lesson == nil {
		return errors.New("lesson not found")
	}

	return s.lessonRepo.Delete(ctx, lessonID)
}

func (s *LessonService) ReorderLessons(ctx context.Context, sectionID uuid.UUID, req dto.ReorderDTO) error {
	if err := s.validateSection(ctx, sectionID); err != nil {
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

	count, err := s.lessonRepo.CountByIDsAndSection(ctx, ids, sectionID)
	if err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return errors.New("one or more lessons do not belong to this section")
	}

	return s.lessonRepo.Reorder(ctx, items)
}

func (s *LessonService) toLessonResponseDTO(lesson *model.Lesson, contents []model.LessonContent) *dto.LessonResponseDTO {
	resp := &dto.LessonResponseDTO{
		ID:           lesson.ID,
		SectionID:    lesson.SectionID,
		Title:        lesson.Title,
		Description:  lesson.Description,
		DisplayOrder: lesson.DisplayOrder,
		DurationMins: lesson.DurationMins,
		IsPreview:    lesson.IsPreview,
		IsMandatory:  lesson.IsMandatory,
		CreatedAt:    lesson.CreatedAt,
		UpdatedAt:    lesson.UpdatedAt,
	}

	if len(contents) > 0 {
		resp.Contents = make([]dto.LessonContentResponseDTO, len(contents))
		for i, c := range contents {
			resp.Contents[i] = dto.LessonContentResponseDTO{
				ID:           c.ID,
				LessonID:     c.LessonID,
				Type:         c.Type,
				Title:        c.Title,
				VideoURL:     c.VideoURL,
				Duration:     c.Duration,
				DisplayOrder: c.DisplayOrder,
				CreatedAt:    c.CreatedAt,
				UpdatedAt:    c.UpdatedAt,
			}
		}
	}

	return resp
}
