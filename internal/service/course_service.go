package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// ErrNotCourseOwner được handler ánh xạ sang HTTP 403 — dùng chung message với
// pattern "forbidden: not the owner" đã có sẵn ở discussion_service.go/review_service.go.
var ErrNotCourseOwner = errors.New("forbidden: not the owner")

type CourseServiceInterface interface {
	CreateCourse(ctx context.Context, req dto.CreateCourseDTO) (*dto.CourseResponseDTO, error)
	GetAllCourses(ctx context.Context, params dto.CourseFilterParams) (*dto.CourseListResponseDTO, error)
	GetCourseByID(ctx context.Context, id uuid.UUID) (*dto.CourseDetailDTO, error)
	GetCourseBySlug(ctx context.Context, slug string) (*dto.CourseDetailDTO, error)
	UpdateCourse(ctx context.Context, id, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateCourseDTO) (*dto.CourseResponseDTO, error)
	DeleteCourse(ctx context.Context, id, actorUserID uuid.UUID, isAdmin bool) error
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

	slug, err := utils.GenerateUniqueSlug(req.Title, func(slug string) (bool, error) {
		return s.courseRepo.SlugExists(ctx, slug)
	})
	if err != nil {
		return nil, err
	}

	course := &model.Course{
		InstructorID:      req.InstructorID,
		CategoryID:        req.CategoryID,
		Title:             req.Title,
		Slug:              slug,
		ShortDescription:  req.ShortDescription,
		Description:       req.Description,
		ThumbnailURL:      req.ThumbnailURL,
		PreviewVideoURL:   req.PreviewVideoURL,
		Level:             level,
		Language:          language,
		Price:             req.Price,
		DiscountPrice:     req.DiscountPrice,
		DiscountExpiresAt: req.DiscountExpiresAt,
		Requirements:      req.Requirements,
		Objectives:        req.Objectives,
		TargetAudience:    req.TargetAudience,
		Status:            "draft",
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
		CategoryID:   params.CategoryID,
		InstructorID: params.InstructorID,
		Level:        params.Level,
		Status:       params.Status,
		Keyword:      params.Keyword,
		IsFree:       params.IsFree,
		IsFeatured:   params.IsFeatured,
		MinPrice:     params.MinPrice,
		MaxPrice:     params.MaxPrice,
		TagIDs:       params.TagIDs,
		Page:         params.Page,
		PageSize:     params.PageSize,
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
			lessons[j] = s.toLessonResponseDTO(&les, les.Contents)
		}
		sections[i] = dto.SectionResponseDTO{
			ID:           sec.ID,
			CourseID:     sec.CourseID,
			Title:        sec.Title,
			Description:  sec.Description,
			DisplayOrder: sec.DisplayOrder,
			Lessons:      lessons,
			CreatedAt:    sec.CreatedAt,
			UpdatedAt:    sec.UpdatedAt,
		}
	}
	detail.Sections = sections

	return detail, nil
}

// GetCourseBySlug retrieves course detail by its URL slug
func (s *CourseService) GetCourseBySlug(ctx context.Context, slug string) (*dto.CourseDetailDTO, error) {
	course, err := s.courseRepo.GetDetailBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, errors.New("course not found")
	}
	if course.Status != "published" {
		return nil, errors.New("course not found")
	}

	detail := &dto.CourseDetailDTO{
		CourseResponseDTO: *s.toCourseResponseDTO(course),
	}

	sections := make([]dto.SectionResponseDTO, len(course.Sections))
	for i, sec := range course.Sections {
		lessons := make([]dto.LessonResponseDTO, len(sec.Lessons))
		for j, les := range sec.Lessons {
			// Public course detail exposes the syllabus, never protected content URLs.
			lessons[j] = s.toLessonResponseDTO(&les, nil)

			// Phase 1 §2: route nay KHONG co user dang nhap (public, xem truoc khi mua) nen
			// khong biet duoc "bai truoc da completed chua" — chi biet CHAC MOT dieu: nguoi
			// xem CHUA enroll. Bai preview/mien phi luon mo (dung y "bo qua bai preview" cua
			// contract); moi bai con lai khoa voi ly do "not_enrolled". Khong co progress vi
			// khong biet la ai.
			if !les.IsPreview {
				reason := LockReasonNotEnrolled
				lessons[j].Locked = true
				lessons[j].LockReason = &reason
			}
		}
		sections[i] = dto.SectionResponseDTO{
			ID:           sec.ID,
			CourseID:     sec.CourseID,
			Title:        sec.Title,
			Description:  sec.Description,
			DisplayOrder: sec.DisplayOrder,
			Lessons:      lessons,
			CreatedAt:    sec.CreatedAt,
			UpdatedAt:    sec.UpdatedAt,
		}
	}
	detail.Sections = sections

	return detail, nil
}

func (s *CourseService) UpdateCourse(ctx context.Context, id, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateCourseDTO) (*dto.CourseResponseDTO, error) {
	course, err := s.courseRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, errors.New("course not found")
	}
	// C-12 (audit 260909): trước đây bất kỳ user đăng nhập nào cũng UpdateCourse được khóa
	// học của người khác vì service không nhận/so sánh userID với course.InstructorID.
	// Vòng 2: cho phép SYSTEM_ADMIN override (isAdmin tính sẵn ở handler qua PermissionChecker)
	// để kiểm duyệt/sửa khóa học vi phạm của giảng viên khác.
	if course.InstructorID != actorUserID && !isAdmin {
		return nil, ErrNotCourseOwner
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
		newSlug, err := utils.GenerateUniqueSlug(*req.Title, func(slug string) (bool, error) {
			// Allow the current course to keep its own slug
			if slug == course.Slug {
				return false, nil
			}
			return s.courseRepo.SlugExists(ctx, slug)
		})
		if err != nil {
			return nil, err
		}
		course.Slug = newSlug
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
	if req.DiscountPrice != nil {
		course.DiscountPrice = req.DiscountPrice
	}
	if req.DiscountExpiresAt != nil {
		course.DiscountExpiresAt = req.DiscountExpiresAt
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
	if req.Sequential != nil {
		course.Sequential = *req.Sequential
	}
	if req.MinVideoPct != nil {
		course.MinVideoPct = *req.MinVideoPct
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

func (s *CourseService) DeleteCourse(ctx context.Context, id, actorUserID uuid.UUID, isAdmin bool) error {
	course, err := s.courseRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if course == nil {
		return errors.New("course not found")
	}
	// C-12 (audit 260909): tương tự UpdateCourse — chỉ giảng viên tạo khóa học hoặc
	// SYSTEM_ADMIN (vòng 2) mới được xóa.
	if course.InstructorID != actorUserID && !isAdmin {
		return ErrNotCourseOwner
	}
	return s.courseRepo.Delete(ctx, id)
}

func (s *CourseService) toCourseResponseDTO(course *model.Course) *dto.CourseResponseDTO {
	resp := &dto.CourseResponseDTO{
		ID:                course.ID,
		InstructorID:      course.InstructorID,
		CategoryID:        course.CategoryID,
		Title:             course.Title,
		Slug:              course.Slug,
		ShortDescription:  course.ShortDescription,
		Description:       course.Description,
		ThumbnailURL:      course.ThumbnailURL,
		PreviewVideoURL:   course.PreviewVideoURL,
		Level:             course.Level,
		Language:          course.Language,
		Price:             course.Price,
		DiscountPrice:     course.DiscountPrice,
		DiscountExpiresAt: course.DiscountExpiresAt,
		TotalDurationMins: course.TotalDurationMins,
		TotalLessons:      course.TotalLessons,
		TotalStudents:     course.TotalStudents,
		AverageRating:     course.AverageRating,
		TotalReviews:      course.TotalReviews,
		Requirements:      course.Requirements,
		Objectives:        course.Objectives,
		TargetAudience:    course.TargetAudience,
		Status:            course.Status,
		PublishedAt:       course.PublishedAt,
		IsFeatured:        course.IsFeatured,
		IsFree:            course.IsFree,
		CreatedAt:         course.CreatedAt,
		UpdatedAt:         course.UpdatedAt,
		Sequential:        course.Sequential,
		MinVideoPct:       course.MinVideoPct,
	}
	if resp.MinVideoPct <= 0 {
		// Dong cu duoc AutoMigrate them cot voi gia tri 0 — tra ve dung nguong THUC TE dang
		// duoc dung (xem defaultMinVideoPct, enrollment_service.go) thay vi mot con so sai.
		resp.MinVideoPct = defaultMinVideoPct
	}

	if course.Instructor.ID != uuid.Nil {
		name := course.Instructor.UserName
		if course.Instructor.FullName != nil && *course.Instructor.FullName != "" {
			name = *course.Instructor.FullName
		}
		resp.Instructor = &dto.CourseInstructorDTO{
			ID:        course.Instructor.ID,
			Name:      name,
			AvatarURL: course.Instructor.AvatarURL,
			Bio:       course.Instructor.Bio,
		}
	}

	if course.Category != nil {
		resp.Category = &dto.CategoryResponseDTO{
			ID:           course.Category.ID,
			ParentID:     course.Category.ParentID,
			Name:         course.Category.Name,
			Slug:         course.Category.Slug,
			Description:  course.Category.Description,
			IconURL:      course.Category.IconURL,
			DisplayOrder: course.Category.DisplayOrder,
			IsActive:     course.Category.IsActive,
			CreatedAt:    course.Category.CreatedAt,
			UpdatedAt:    course.Category.UpdatedAt,
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

func (s *CourseService) toLessonResponseDTO(lesson *model.Lesson, contents []model.LessonContent) dto.LessonResponseDTO {
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
				ID:       c.ID,
				LessonID: c.LessonID,
				Type:     c.Type,
				Title:    c.Title,
				VideoURL: c.VideoURL,
				Duration: c.Duration,
				// N10 (review vòng 2, từ review web): xem chú thích tại model.LessonContent.
				LivestreamSessionID: c.LivestreamSessionID,
				DisplayOrder:        c.DisplayOrder,
				CreatedAt:           c.CreatedAt,
				UpdatedAt:           c.UpdatedAt,
			}
		}
	}

	return resp
}
