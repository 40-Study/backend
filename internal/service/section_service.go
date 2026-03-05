package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type SectionServiceInterface interface {
	CreateSection(ctx context.Context, courseID uuid.UUID, req dto.CreateSectionDTO) (*dto.SectionResponseDTO, error)
	GetAllSections(ctx context.Context, courseID uuid.UUID) ([]dto.SectionResponseDTO, error)
	UpdateSection(ctx context.Context, courseID, sectionID uuid.UUID, req dto.UpdateSectionDTO) (*dto.SectionResponseDTO, error)
	DeleteSection(ctx context.Context, courseID, sectionID uuid.UUID) error
	ReorderSections(ctx context.Context, courseID uuid.UUID, req dto.ReorderDTO) error
}

type SectionService struct {
	sectionRepo repository.SectionRepositoryInterface
	courseRepo  repository.CourseRepositoryInterface
}

func NewSectionService(
	sectionRepo repository.SectionRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
) *SectionService {
	return &SectionService{
		sectionRepo: sectionRepo,
		courseRepo:  courseRepo,
	}
}

func (s *SectionService) CreateSection(ctx context.Context, courseID uuid.UUID, req dto.CreateSectionDTO) (*dto.SectionResponseDTO, error) {
	exists, err := s.courseRepo.Exists(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("course not found")
	}

	maxOrder, err := s.sectionRepo.GetMaxDisplayOrder(ctx, courseID)
	if err != nil {
		return nil, err
	}

	section := &model.Section{
		CourseID:     courseID,
		Title:        req.Title,
		Description:  req.Description,
		DisplayOrder: maxOrder + 1,
	}

	if err := s.sectionRepo.Create(ctx, section); err != nil {
		return nil, err
	}

	return s.toSectionResponseDTO(section), nil
}

func (s *SectionService) GetAllSections(ctx context.Context, courseID uuid.UUID) ([]dto.SectionResponseDTO, error) {
	exists, err := s.courseRepo.Exists(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("course not found")
	}

	sections, err := s.sectionRepo.GetAllByCourseID(ctx, courseID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.SectionResponseDTO, len(sections))
	for i, sec := range sections {
		d := s.toSectionResponseDTO(&sec)
		lessons := make([]dto.LessonResponseDTO, len(sec.Lessons))
		for j, les := range sec.Lessons {
			lessons[j] = toLessonResponseDTO(&les)
		}
		d.Lessons = lessons
		result[i] = *d
	}

	return result, nil
}

func (s *SectionService) UpdateSection(ctx context.Context, courseID, sectionID uuid.UUID, req dto.UpdateSectionDTO) (*dto.SectionResponseDTO, error) {
	belongs, err := s.sectionRepo.BelongsToCourse(ctx, sectionID, courseID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("section not found in this course")
	}

	section, err := s.sectionRepo.GetByID(ctx, sectionID)
	if err != nil {
		return nil, err
	}
	if section == nil {
		return nil, errors.New("section not found")
	}

	if req.Title != nil {
		section.Title = *req.Title
	}
	if req.Description != nil {
		section.Description = req.Description
	}

	if err := s.sectionRepo.Update(ctx, section); err != nil {
		return nil, err
	}

	return s.toSectionResponseDTO(section), nil
}

func (s *SectionService) DeleteSection(ctx context.Context, courseID, sectionID uuid.UUID) error {
	belongs, err := s.sectionRepo.BelongsToCourse(ctx, sectionID, courseID)
	if err != nil {
		return err
	}
	if !belongs {
		return errors.New("section not found in this course")
	}

	return s.sectionRepo.Delete(ctx, sectionID)
}

func (s *SectionService) ReorderSections(ctx context.Context, courseID uuid.UUID, req dto.ReorderDTO) error {
	exists, err := s.courseRepo.Exists(ctx, courseID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("course not found")
	}

	items := make([]repository.ReorderItem, len(req.Items))
	for i, item := range req.Items {
		belongs, err := s.sectionRepo.BelongsToCourse(ctx, item.ID, courseID)
		if err != nil {
			return err
		}
		if !belongs {
			return errors.New("section " + item.ID.String() + " does not belong to this course")
		}
		items[i] = repository.ReorderItem{
			ID:           item.ID,
			DisplayOrder: item.DisplayOrder,
		}
	}

	return s.sectionRepo.Reorder(ctx, items)
}

func (s *SectionService) toSectionResponseDTO(section *model.Section) *dto.SectionResponseDTO {
	return &dto.SectionResponseDTO{
		ID:           section.ID,
		CourseID:     section.CourseID,
		Title:        section.Title,
		Description:  section.Description,
		DisplayOrder: section.DisplayOrder,
		CreatedAt:    section.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    section.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
