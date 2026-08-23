package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type ClassLessonContentServiceInterface interface {
	AssignClassToContent(ctx context.Context, contentID uuid.UUID, userID uuid.UUID, req dto.AssignClassToContentDTO) (*dto.ClassLessonContentResponseDTO, error)
	UpdateClassContentSchedule(ctx context.Context, contentID, classID uuid.UUID, req dto.UpdateClassContentScheduleDTO) (*dto.ClassLessonContentResponseDTO, error)
	RemoveClassFromContent(ctx context.Context, contentID, classID uuid.UUID) error
	GetClassesForContent(ctx context.Context, contentID uuid.UUID, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error)
	GetContentScheduleForClass(ctx context.Context, classID uuid.UUID, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error)
	BulkAssignClassesToContent(ctx context.Context, contentID uuid.UUID, userID uuid.UUID, req dto.BulkAssignClassesToContentDTO) ([]dto.ClassLessonContentResponseDTO, error)
}

type ClassLessonContentService struct {
	clcRepo        repository.ClassLessonContentRepositoryInterface
	classRepo      repository.ClassRepositoryInterface
	lessonRepo     repository.LessonRepositoryInterface
	enrollmentRepo repository.EnrollmentRepositoryInterface
	livestreamSvc  LivestreamServiceInterface
}

func NewClassLessonContentService(
	clcRepo repository.ClassLessonContentRepositoryInterface,
	classRepo repository.ClassRepositoryInterface,
	lessonRepo repository.LessonRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	livestreamSvc LivestreamServiceInterface,
) *ClassLessonContentService {
	return &ClassLessonContentService{
		clcRepo:        clcRepo,
		classRepo:      classRepo,
		lessonRepo:     lessonRepo,
		enrollmentRepo: enrollmentRepo,
		livestreamSvc:  livestreamSvc,
	}
}

func (s *ClassLessonContentService) AssignClassToContent(ctx context.Context, contentID uuid.UUID, userID uuid.UUID, req dto.AssignClassToContentDTO) (*dto.ClassLessonContentResponseDTO, error) {
	// Validate content exists
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	// Validate class exists
	class, err := s.classRepo.GetByID(ctx, req.ClassID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// Validate class belongs to the same course as the content
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, content.LessonID)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve course from lesson: %w", err)
	}
	if class.CourseID == nil || *class.CourseID != courseID {
		return nil, errors.New("class does not belong to this course")
	}

	// Check duplicate
	exists, err := s.clcRepo.Exists(ctx, req.ClassID, contentID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("class is already assigned to this content")
	}

	// Parse dates
	clc := &model.ClassLessonContent{
		ID:              uuid.New(),
		ClassID:         req.ClassID,
		LessonContentID: contentID,
		Status:          "scheduled",
	}

	if err := s.parseDates(req.OpenDate, req.DueDate, req.ScheduledAt, req.EndAt, clc); err != nil {
		return nil, err
	}

	// Livestream conflict check
	if content.Type == "livestream" && clc.ScheduledAt != nil && clc.EndAt != nil {
		overlap, err := s.clcRepo.HasOverlappingLivestream(ctx, req.ClassID, *clc.ScheduledAt, *clc.EndAt, nil)
		if err != nil {
			return nil, err
		}
		if overlap {
			return nil, errors.New("class has an overlapping livestream session at this time")
		}
	}

	if err := s.clcRepo.Create(ctx, clc); err != nil {
		return nil, err
	}

	// Auto-create LivestreamSession for livestream content
	if content.Type == "livestream" && clc.ScheduledAt != nil {
		s.createLivestreamSession(ctx, clc, content, class, courseID, userID)
	}

	// Reload with preloaded relationships
	created, err := s.clcRepo.GetByClassAndContent(ctx, req.ClassID, contentID)
	if err != nil {
		return nil, err
	}

	return s.toResponseDTO(created), nil
}

func (s *ClassLessonContentService) UpdateClassContentSchedule(ctx context.Context, contentID, classID uuid.UUID, req dto.UpdateClassContentScheduleDTO) (*dto.ClassLessonContentResponseDTO, error) {
	clc, err := s.clcRepo.GetByClassAndContent(ctx, classID, contentID)
	if err != nil {
		return nil, err
	}
	if clc == nil {
		return nil, errors.New("class-content assignment not found")
	}

	if req.OpenDate != nil {
		t, err := time.Parse(time.RFC3339, *req.OpenDate)
		if err != nil {
			return nil, errors.New("invalid open_date format, expected RFC3339")
		}
		clc.OpenDate = &t
	}
	if req.DueDate != nil {
		t, err := time.Parse(time.RFC3339, *req.DueDate)
		if err != nil {
			return nil, errors.New("invalid due_date format, expected RFC3339")
		}
		clc.DueDate = &t
	}
	if req.ScheduledAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ScheduledAt)
		if err != nil {
			return nil, errors.New("invalid scheduled_at format, expected RFC3339")
		}
		clc.ScheduledAt = &t
	}
	if req.EndAt != nil {
		t, err := time.Parse(time.RFC3339, *req.EndAt)
		if err != nil {
			return nil, errors.New("invalid end_at format, expected RFC3339")
		}
		clc.EndAt = &t
	}
	if req.Status != nil {
		clc.Status = *req.Status
	}

	// Livestream conflict check if times changed
	if clc.LessonContent.Type == "livestream" && clc.ScheduledAt != nil && clc.EndAt != nil {
		overlap, err := s.clcRepo.HasOverlappingLivestream(ctx, classID, *clc.ScheduledAt, *clc.EndAt, &contentID)
		if err != nil {
			return nil, err
		}
		if overlap {
			return nil, errors.New("class has an overlapping livestream session at this time")
		}
	}

	if err := s.clcRepo.Update(ctx, clc); err != nil {
		return nil, err
	}

	// Reload
	updated, err := s.clcRepo.GetByClassAndContent(ctx, classID, contentID)
	if err != nil {
		return nil, err
	}

	return s.toResponseDTO(updated), nil
}

func (s *ClassLessonContentService) RemoveClassFromContent(ctx context.Context, contentID, classID uuid.UUID) error {
	clc, err := s.clcRepo.GetByClassAndContent(ctx, classID, contentID)
	if err != nil {
		return err
	}
	if clc == nil {
		return errors.New("class-content assignment not found")
	}

	return s.clcRepo.Delete(ctx, classID, contentID)
}

func (s *ClassLessonContentService) GetClassesForContent(ctx context.Context, contentID uuid.UUID, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error) {
	// Validate content exists
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	items, total, err := s.clcRepo.GetByContentID(ctx, contentID, page, pageSize)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.ClassLessonContentResponseDTO, len(items))
	for i, item := range items {
		dtos[i] = *s.toResponseDTO(&item)
	}

	return &dto.ClassLessonContentListResponseDTO{
		Items:    dtos,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ClassLessonContentService) GetContentScheduleForClass(ctx context.Context, classID uuid.UUID, page, pageSize int) (*dto.ClassLessonContentListResponseDTO, error) {
	// Validate class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	items, total, err := s.clcRepo.GetByClassID(ctx, classID, page, pageSize)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.ClassLessonContentResponseDTO, len(items))
	for i, item := range items {
		dtos[i] = *s.toResponseDTO(&item)
	}

	return &dto.ClassLessonContentListResponseDTO{
		Items:    dtos,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ClassLessonContentService) BulkAssignClassesToContent(ctx context.Context, contentID uuid.UUID, userID uuid.UUID, req dto.BulkAssignClassesToContentDTO) ([]dto.ClassLessonContentResponseDTO, error) {
	// Validate content exists
	content, err := s.lessonRepo.GetContentByID(ctx, contentID)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, errors.New("content not found")
	}

	// Resolve courseID
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, content.LessonID)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve course from lesson: %w", err)
	}

	// Determine class list
	var classIDs []uuid.UUID
	if len(req.ClassIDs) > 0 {
		classIDs = req.ClassIDs
	} else {
		// Get all classes in course
		classes, err := s.classRepo.GetByCourseID(ctx, courseID)
		if err != nil {
			return nil, err
		}
		for _, c := range classes {
			classIDs = append(classIDs, c.ID)
		}
	}

	if len(classIDs) == 0 {
		return nil, errors.New("no classes found for this course")
	}

	var clcs []*model.ClassLessonContent
	for _, classID := range classIDs {
		// Validate class belongs to course
		class, err := s.classRepo.GetByID(ctx, classID)
		if err != nil {
			return nil, err
		}
		if class == nil {
			return nil, fmt.Errorf("class not found: %s", classID)
		}
		if class.CourseID == nil || *class.CourseID != courseID {
			return nil, fmt.Errorf("class %s does not belong to this course", classID)
		}

		// Skip if already assigned
		exists, err := s.clcRepo.Exists(ctx, classID, contentID)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}

		clc := &model.ClassLessonContent{
			ID:              uuid.New(),
			ClassID:         classID,
			LessonContentID: contentID,
			Status:          "scheduled",
		}

		if err := s.parseDates(req.OpenDate, req.DueDate, req.ScheduledAt, req.EndAt, clc); err != nil {
			return nil, err
		}

		// Livestream conflict check
		if content.Type == "livestream" && clc.ScheduledAt != nil && clc.EndAt != nil {
			overlap, err := s.clcRepo.HasOverlappingLivestream(ctx, classID, *clc.ScheduledAt, *clc.EndAt, nil)
			if err != nil {
				return nil, err
			}
			if overlap {
				return nil, fmt.Errorf("class %s has an overlapping livestream session at this time", class.Name)
			}
		}

		clcs = append(clcs, clc)
	}

	if len(clcs) == 0 {
		return nil, errors.New("all classes are already assigned to this content")
	}

	if err := s.clcRepo.CreateBatch(ctx, clcs); err != nil {
		return nil, err
	}

	// Build response
	result := make([]dto.ClassLessonContentResponseDTO, len(clcs))
	for i, clc := range clcs {
		loaded, err := s.clcRepo.GetByClassAndContent(ctx, clc.ClassID, contentID)
		if err != nil || loaded == nil {
			continue
		}
		result[i] = *s.toResponseDTO(loaded)
	}

	return result, nil
}

// Helpers

func (s *ClassLessonContentService) parseDates(openDate, dueDate, scheduledAt, endAt *string, clc *model.ClassLessonContent) error {
	if openDate != nil {
		t, err := time.Parse(time.RFC3339, *openDate)
		if err != nil {
			return errors.New("invalid open_date format, expected RFC3339")
		}
		clc.OpenDate = &t
	}
	if dueDate != nil {
		t, err := time.Parse(time.RFC3339, *dueDate)
		if err != nil {
			return errors.New("invalid due_date format, expected RFC3339")
		}
		clc.DueDate = &t
	}
	if scheduledAt != nil {
		t, err := time.Parse(time.RFC3339, *scheduledAt)
		if err != nil {
			return errors.New("invalid scheduled_at format, expected RFC3339")
		}
		clc.ScheduledAt = &t
	}
	if endAt != nil {
		t, err := time.Parse(time.RFC3339, *endAt)
		if err != nil {
			return errors.New("invalid end_at format, expected RFC3339")
		}
		clc.EndAt = &t
	}
	return nil
}

func (s *ClassLessonContentService) createLivestreamSession(ctx context.Context, clc *model.ClassLessonContent, content *model.LessonContent, class *model.Class, courseID uuid.UUID, userID uuid.UUID) {
	title := "Livestream"
	if content.Title != nil {
		title = *content.Title
	}
	scheduledDate := ""
	startTime := ""
	if clc.ScheduledAt != nil {
		scheduledDate = clc.ScheduledAt.Format("2006-01-02")
		startTime = clc.ScheduledAt.Format("15:04")
	}

	livestreamReq := dto.CreateLivestreamDTO{
		Title:           fmt.Sprintf("%s - %s", title, class.Name),
		HostID:          userID.String(),
		ClassID:         clc.ClassID.String(),
		CourseID:        courseID.String(),
		LessonContentID: clc.LessonContentID.String(),
		ScheduledDate:   scheduledDate,
		StartTime:       startTime,
		DurationMinutes: 60,
		Platform:        "40study",
		EnableRecording: true,
		MaxViewers:      1000,
	}

	_, _ = s.livestreamSvc.Create(ctx, livestreamReq)
}

func (s *ClassLessonContentService) toResponseDTO(clc *model.ClassLessonContent) *dto.ClassLessonContentResponseDTO {
	resp := &dto.ClassLessonContentResponseDTO{
		ID:              clc.ID,
		ClassID:         clc.ClassID,
		LessonContentID: clc.LessonContentID,
		OpenDate:        clc.OpenDate,
		DueDate:         clc.DueDate,
		ScheduledAt:     clc.ScheduledAt,
		EndAt:           clc.EndAt,
		Status:          clc.Status,
		CreatedAt:       clc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       clc.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	// From preloaded relationships
	resp.ClassName = clc.Class.Name
	resp.ContentType = clc.LessonContent.Type
	resp.ContentTitle = clc.LessonContent.Title

	return resp
}
