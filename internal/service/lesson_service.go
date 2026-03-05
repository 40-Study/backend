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
	CreateLesson(ctx context.Context, courseID, sectionID uuid.UUID, req dto.CreateLessonDTO) (*dto.LessonResponseDTO, error)
	GetAllLessons(ctx context.Context, courseID, sectionID uuid.UUID) ([]dto.LessonResponseDTO, error)
	UpdateLesson(ctx context.Context, courseID, sectionID, lessonID uuid.UUID, req dto.UpdateLessonDTO) (*dto.LessonResponseDTO, error)
	DeleteLesson(ctx context.Context, courseID, sectionID, lessonID uuid.UUID) error
	ReorderLessons(ctx context.Context, courseID, sectionID uuid.UUID, req dto.ReorderDTO) error
}

type LessonService struct {
	lessonRepo  repository.LessonRepositoryInterface
	sectionRepo repository.SectionRepositoryInterface
	courseRepo  repository.CourseRepositoryInterface
}

func NewLessonService(
	lessonRepo repository.LessonRepositoryInterface,
	sectionRepo repository.SectionRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
) *LessonService {
	return &LessonService{
		lessonRepo:  lessonRepo,
		sectionRepo: sectionRepo,
		courseRepo:  courseRepo,
	}
}

func (s *LessonService) validateCourseSection(ctx context.Context, courseID, sectionID uuid.UUID) error {
	exists, err := s.courseRepo.Exists(ctx, courseID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("course not found")
	}

	belongs, err := s.sectionRepo.BelongsToCourse(ctx, sectionID, courseID)
	if err != nil {
		return err
	}
	if !belongs {
		return errors.New("section not found in this course")
	}
	return nil
}

func (s *LessonService) CreateLesson(ctx context.Context, courseID, sectionID uuid.UUID, req dto.CreateLessonDTO) (*dto.LessonResponseDTO, error) {
	if err := s.validateCourseSection(ctx, courseID, sectionID); err != nil {
		return nil, err
	}

	maxOrder, err := s.lessonRepo.GetMaxDisplayOrder(ctx, sectionID)
	if err != nil {
		return nil, err
	}

	lesson := &model.Lesson{
		ID:           uuid.New(),
		SectionID:    sectionID,
		Title:        req.Title,
		Description:  req.Description,
		ContentType:  req.ContentType,
		DisplayOrder: maxOrder + 1,
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

	result := toLessonResponseDTO(lesson)
	return &result, nil
}

func (s *LessonService) GetAllLessons(ctx context.Context, courseID, sectionID uuid.UUID) ([]dto.LessonResponseDTO, error) {
	if err := s.validateCourseSection(ctx, courseID, sectionID); err != nil {
		return nil, err
	}

	lessons, err := s.lessonRepo.GetAllBySectionID(ctx, sectionID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.LessonResponseDTO, len(lessons))
	for i, les := range lessons {
		result[i] = toLessonResponseDTO(&les)
	}

	return result, nil
}

func (s *LessonService) UpdateLesson(ctx context.Context, courseID, sectionID, lessonID uuid.UUID, req dto.UpdateLessonDTO) (*dto.LessonResponseDTO, error) {
	if err := s.validateCourseSection(ctx, courseID, sectionID); err != nil {
		return nil, err
	}

	belongs, err := s.lessonRepo.BelongsToSection(ctx, lessonID, sectionID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("lesson not found in this section")
	}

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
	if req.ContentType != nil {
		lesson.ContentType = *req.ContentType
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

	result := toLessonResponseDTO(lesson)
	return &result, nil
}

func (s *LessonService) DeleteLesson(ctx context.Context, courseID, sectionID, lessonID uuid.UUID) error {
	if err := s.validateCourseSection(ctx, courseID, sectionID); err != nil {
		return err
	}

	belongs, err := s.lessonRepo.BelongsToSection(ctx, lessonID, sectionID)
	if err != nil {
		return err
	}
	if !belongs {
		return errors.New("lesson not found in this section")
	}

	return s.lessonRepo.Delete(ctx, lessonID)
}

func (s *LessonService) ReorderLessons(ctx context.Context, courseID, sectionID uuid.UUID, req dto.ReorderDTO) error {
	if err := s.validateCourseSection(ctx, courseID, sectionID); err != nil {
		return err
	}

	items := make([]repository.ReorderItem, len(req.Items))
	for i, item := range req.Items {
		belongs, err := s.lessonRepo.BelongsToSection(ctx, item.ID, sectionID)
		if err != nil {
			return err
		}
		if !belongs {
			return errors.New("lesson " + item.ID.String() + " does not belong to this section")
		}
		items[i] = repository.ReorderItem{
			ID:           item.ID,
			DisplayOrder: item.DisplayOrder,
		}
	}

	return s.lessonRepo.Reorder(ctx, items)
}
