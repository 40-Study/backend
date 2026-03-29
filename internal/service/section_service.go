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
	GetSectionByID(ctx context.Context, sectionID uuid.UUID) (*dto.SectionResponseDTO, error)
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

func (s *SectionService) validateCourse(ctx context.Context, courseID uuid.UUID) error {
	exists, err := s.courseRepo.Exists(ctx, courseID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("course not found")
	}
	return nil
}

func (s *SectionService) CreateSection(ctx context.Context, courseID uuid.UUID, req dto.CreateSectionDTO) (*dto.SectionResponseDTO, error) {
	if err := s.validateCourse(ctx, courseID); err != nil {
		return nil, err
	}

	maxOrder, err := s.sectionRepo.GetMaxDisplayOrder(ctx, courseID)
	if err != nil {
		return nil, err
	}

	displayOrder := maxOrder + 1
	if req.DisplayOrder > 0 {
		displayOrder = req.DisplayOrder
	}

	section := &model.Section{
		CourseID:     courseID,
		Title:        req.Title,
		Description:  req.Description,
		DisplayOrder: displayOrder,
	}

	if err := s.sectionRepo.Create(ctx, section); err != nil {
		return nil, err
	}

	return s.toSectionResponseDTO(section, nil), nil
}

func (s *SectionService) GetAllSections(ctx context.Context, courseID uuid.UUID) ([]dto.SectionResponseDTO, error) {
	if err := s.validateCourse(ctx, courseID); err != nil {
		return nil, err
	}

	sections, err := s.sectionRepo.GetAllByCourseID(ctx, courseID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.SectionResponseDTO, len(sections))
	for i, sec := range sections {
		lessons := make([]dto.LessonResponseDTO, len(sec.Lessons))
		for j, les := range sec.Lessons {
			lessons[j] = s.toLessonResponseDTO(&les, nil, nil)
		}
		result[i] = *s.toSectionResponseDTO(&sec, lessons)
	}

	return result, nil
}

func (s *SectionService) GetSectionByID(ctx context.Context, sectionID uuid.UUID) (*dto.SectionResponseDTO, error) {
	section, err := s.sectionRepo.GetByID(ctx, sectionID)
	if err != nil {
		return nil, err
	}
	if section == nil {
		return nil, errors.New("section not found")
	}

	lessons := make([]dto.LessonResponseDTO, len(section.Lessons))
	for j, les := range section.Lessons {
		lessons[j] = s.toLessonResponseDTO(&les, nil, nil)
	}

	return s.toSectionResponseDTO(section, lessons), nil
}

func (s *SectionService) UpdateSection(ctx context.Context, courseID, sectionID uuid.UUID, req dto.UpdateSectionDTO) (*dto.SectionResponseDTO, error) {
	if err := s.validateCourse(ctx, courseID); err != nil {
		return nil, err
	}

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
	if req.DisplayOrder != nil {
		section.DisplayOrder = *req.DisplayOrder
	}

	if err := s.sectionRepo.Update(ctx, section); err != nil {
		return nil, err
	}

	lessons := make([]dto.LessonResponseDTO, len(section.Lessons))
	for j, les := range section.Lessons {
		lessons[j] = s.toLessonResponseDTO(&les, nil, nil)
	}

	return s.toSectionResponseDTO(section, lessons), nil
}

func (s *SectionService) DeleteSection(ctx context.Context, courseID, sectionID uuid.UUID) error {
	if err := s.validateCourse(ctx, courseID); err != nil {
		return err
	}

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
	if err := s.validateCourse(ctx, courseID); err != nil {
		return err
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

func (s *SectionService) toSectionResponseDTO(section *model.Section, lessons []dto.LessonResponseDTO) *dto.SectionResponseDTO {
	return &dto.SectionResponseDTO{
		ID:           section.ID,
		CourseID:     section.CourseID,
		Title:        section.Title,
		Description:  section.Description,
		DisplayOrder: section.DisplayOrder,
		Lessons:      lessons,
		CreatedAt:    section.CreatedAt,
		UpdatedAt:    section.UpdatedAt,
	}
}

func (s *SectionService) toLessonResponseDTO(lesson *model.Lesson, contents []model.LessonContent, sessions []model.LessonSession) dto.LessonResponseDTO {
	resp := dto.LessonResponseDTO{
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
				StreamID:     c.StreamID,
				DisplayOrder: c.DisplayOrder,
				CreatedAt:    c.CreatedAt,
				UpdatedAt:    c.UpdatedAt,
			}
		}
	}

	if len(sessions) > 0 {
		resp.Sessions = make([]dto.LessonSessionResponseDTO, len(sessions))
		for i, sess := range sessions {
			resp.Sessions[i] = dto.LessonSessionResponseDTO{
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
	}

	return resp
}
