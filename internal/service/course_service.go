package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type CourseServiceInterface interface {
	CreateCourse(ctx context.Context, req dto.CreateCourseDTO) (*dto.CourseResponseDTO, error)
	GetAllCourses(ctx context.Context, params dto.CourseFilterParams) (*dto.CourseListResponseDTO, error)
	GetCourseByID(ctx context.Context, id uuid.UUID) (*dto.CourseDetailDTO, error)
	UpdateCourse(ctx context.Context, id uuid.UUID, req dto.UpdateCourseDTO) (*dto.CourseResponseDTO, error)
	DeleteCourse(ctx context.Context, id uuid.UUID) error
}

type CourseService struct {
	courseRepo   repository.CourseRepositoryInterface
	categoryRepo repository.CategoryRepositoryInterface
	tagRepo      repository.TagRepositoryInterface
}

func NewCourseService(
	courseRepo repository.CourseRepositoryInterface,
	categoryRepo repository.CategoryRepositoryInterface,
	tagRepo repository.TagRepositoryInterface,
) *CourseService {
	return &CourseService{
		courseRepo:   courseRepo,
		categoryRepo: categoryRepo,
		tagRepo:      tagRepo,
	}
}

func (s *CourseService) CreateCourse(ctx context.Context, req dto.CreateCourseDTO) (*dto.CourseResponseDTO, error) {
	if req.CategoryID != nil {
		exists, err := s.categoryRepo.Exists(ctx, *req.CategoryID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("category not found")
		}
	}

	level := req.Level
	if level == "" {
		level = "beginner"
	}
	language := req.Language
	if language == "" {
		language = "vi"
	}
	price := decimal.NewFromInt(0)
	if req.Price != nil {
		price = *req.Price
	}

	course := &model.Course{
		InstructorID:     req.InstructorID,
		CategoryID:       req.CategoryID,
		Title:            req.Title,
		Slug:             utils.GenerateSlug(req.Title),
		ShortDescription: req.ShortDescription,
		Description:      req.Description,
		ThumbnailURL:     req.ThumbnailURL,
		PreviewVideoURL:  req.PreviewVideoURL,
		Level:            level,
		Language:         language,
		Price:            price,
		Requirements:     req.Requirements,
		Objectives:       req.Objectives,
		TargetAudience:   req.TargetAudience,
		Status:           "draft",
	}

	if req.IsFree != nil {
		course.IsFree = *req.IsFree
	}

	if err := s.courseRepo.Create(ctx, course); err != nil {
		return nil, err
	}

	if len(req.TagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(ctx, req.TagIDs)
		if err != nil {
			return nil, err
		}
		if err := s.courseRepo.ReplaceTags(ctx, course, tags); err != nil {
			return nil, err
		}
		course.Tags = tags
	}

	return s.toCourseResponseDTO(course), nil
}

func (s *CourseService) GetAllCourses(ctx context.Context, params dto.CourseFilterParams) (*dto.CourseListResponseDTO, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		params.PageSize = 20
	}

	dbParams := repository.CourseFilterDBParams{
		CategoryID: params.CategoryID,
		Level:      params.Level,
		Status:     params.Status,
		Keyword:    params.Keyword,
		IsFree:     params.IsFree,
		MinPrice:   params.MinPrice,
		MaxPrice:   params.MaxPrice,
		Page:       params.Page,
		PageSize:   params.PageSize,
	}

	courses, total, err := s.courseRepo.GetAll(ctx, dbParams)
	if err != nil {
		return nil, err
	}

	courseDTOs := make([]dto.CourseResponseDTO, len(courses))
	for i := range courses {
		courseDTOs[i] = *s.toCourseResponseDTO(&courses[i])
	}

	return &dto.CourseListResponseDTO{
		Courses:  courseDTOs,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

func (s *CourseService) GetCourseByID(ctx context.Context, id uuid.UUID) (*dto.CourseDetailDTO, error) {
	course, err := s.courseRepo.GetDetailByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, errors.New("course not found")
	}

	detail := &dto.CourseDetailDTO{
		CourseResponseDTO: *s.toCourseResponseDTO(course),
	}

	sections := make([]dto.SectionResponseDTO, len(course.Sections))
	for i, sec := range course.Sections {
		lessons := make([]dto.LessonResponseDTO, len(sec.Lessons))
		for j, les := range sec.Lessons {
			lessons[j] = toLessonResponseDTO(&les)
		}
		sections[i] = dto.SectionResponseDTO{
			ID:           sec.ID,
			CourseID:     sec.CourseID,
			Title:        sec.Title,
			Description:  sec.Description,
			DisplayOrder: sec.DisplayOrder,
			Lessons:      lessons,
			CreatedAt:    sec.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    sec.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	detail.Sections = sections

	return detail, nil
}

func (s *CourseService) UpdateCourse(ctx context.Context, id uuid.UUID, req dto.UpdateCourseDTO) (*dto.CourseResponseDTO, error) {
	course, err := s.courseRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, errors.New("course not found")
	}

	if req.CategoryID != nil {
		exists, err := s.categoryRepo.Exists(ctx, *req.CategoryID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("category not found")
		}
		course.CategoryID = req.CategoryID
	}

	if req.Title != nil {
		course.Title = *req.Title
		course.Slug = utils.GenerateSlug(*req.Title)
	}
	if req.ShortDescription != nil {
		course.ShortDescription = req.ShortDescription
	}
	if req.Description != nil {
		course.Description = req.Description
	}
	if req.ThumbnailURL != nil {
		course.ThumbnailURL = req.ThumbnailURL
	}
	if req.PreviewVideoURL != nil {
		course.PreviewVideoURL = req.PreviewVideoURL
	}
	if req.Level != nil {
		course.Level = *req.Level
	}
	if req.Language != nil {
		course.Language = *req.Language
	}
	if req.Price != nil {
		course.Price = *req.Price
	}
	if req.Status != nil {
		course.Status = *req.Status
	}
	if req.Requirements != nil {
		course.Requirements = req.Requirements
	}
	if req.Objectives != nil {
		course.Objectives = req.Objectives
	}
	if req.TargetAudience != nil {
		course.TargetAudience = req.TargetAudience
	}
	if req.IsFree != nil {
		course.IsFree = *req.IsFree
	}
	if req.IsFeatured != nil {
		course.IsFeatured = *req.IsFeatured
	}

	if err := s.courseRepo.Update(ctx, course); err != nil {
		return nil, err
	}

	if req.TagIDs != nil {
		tags, err := s.tagRepo.GetByIDs(ctx, req.TagIDs)
		if err != nil {
			return nil, err
		}
		if err := s.courseRepo.ReplaceTags(ctx, course, tags); err != nil {
			return nil, err
		}
		course.Tags = tags
	}

	return s.toCourseResponseDTO(course), nil
}

func (s *CourseService) DeleteCourse(ctx context.Context, id uuid.UUID) error {
	course, err := s.courseRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if course == nil {
		return errors.New("course not found")
	}
	return s.courseRepo.Delete(ctx, id)
}

func (s *CourseService) toCourseResponseDTO(course *model.Course) *dto.CourseResponseDTO {
	resp := &dto.CourseResponseDTO{
		ID:               course.ID,
		InstructorID:     course.InstructorID,
		CategoryID:       course.CategoryID,
		Title:            course.Title,
		Slug:             course.Slug,
		ShortDescription: course.ShortDescription,
		Description:      course.Description,
		ThumbnailURL:     course.ThumbnailURL,
		PreviewVideoURL:  course.PreviewVideoURL,
		Level:            course.Level,
		Language:         course.Language,
		Price:            course.Price,
		DiscountPrice:    course.DiscountPrice,
		TotalDurationMins: course.TotalDurationMins,
		TotalLessons:     course.TotalLessons,
		TotalStudents:    course.TotalStudents,
		AverageRating:    course.AverageRating,
		TotalReviews:     course.TotalReviews,
		Requirements:     course.Requirements,
		Objectives:       course.Objectives,
		TargetAudience:   course.TargetAudience,
		Status:           course.Status,
		IsFeatured:       course.IsFeatured,
		IsFree:           course.IsFree,
		CreatedAt:        course.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        course.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if course.Category != nil {
		resp.Category = &dto.CategoryResponseDTO{
			ID:   course.Category.ID,
			Name: course.Category.Name,
			Slug: course.Category.Slug,
		}
	}

	if len(course.Tags) > 0 {
		tags := make([]dto.TagResponseDTO, len(course.Tags))
		for i, t := range course.Tags {
			tags[i] = dto.TagResponseDTO{
				ID:   t.ID,
				Name: t.Name,
				Slug: t.Slug,
			}
		}
		resp.Tags = tags
	}

	return resp
}

func toLessonResponseDTO(lesson *model.Lesson) dto.LessonResponseDTO {
	return dto.LessonResponseDTO{
		ID:           lesson.ID,
		SectionID:    lesson.SectionID,
		Title:        lesson.Title,
		Description:  lesson.Description,
		ContentType:  lesson.ContentType,
		DisplayOrder: lesson.DisplayOrder,
		DurationMins: lesson.DurationMins,
		IsPreview:    lesson.IsPreview,
		IsMandatory:  lesson.IsMandatory,
	}
}
