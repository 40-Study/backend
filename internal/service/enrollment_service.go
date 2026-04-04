package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type EnrollmentServiceInterface interface {
	Enroll(ctx context.Context, userID, courseID uuid.UUID) (*dto.EnrollmentResponseDTO, error)
	Unenroll(ctx context.Context, userID, courseID uuid.UUID) error
	GetMyEnrollments(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.EnrollmentListResponseDTO, error)
	GetEnrollmentDetail(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*dto.EnrollmentDetailDTO, error)
	UpdateLessonProgress(ctx context.Context, userID, lessonID uuid.UUID, req dto.UpdateLessonProgressDTO) (*dto.LessonProgressResponseDTO, error)
}

type EnrollmentService struct {
	enrollmentRepo repository.EnrollmentRepositoryInterface
	courseRepo     repository.CourseRepositoryInterface
	lessonRepo    repository.LessonRepositoryInterface
}

func NewEnrollmentService(
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	lessonRepo repository.LessonRepositoryInterface,
) *EnrollmentService {
	return &EnrollmentService{
		enrollmentRepo: enrollmentRepo,
		courseRepo:     courseRepo,
		lessonRepo:    lessonRepo,
	}
}

func (s *EnrollmentService) Enroll(ctx context.Context, userID, courseID uuid.UUID) (*dto.EnrollmentResponseDTO, error) {
	exists, err := s.courseRepo.Exists(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("course not found")
	}

	existing, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("already enrolled in this course")
	}

	enrollment := &model.Enrollment{
		UserID:     userID,
		CourseID:   courseID,
		EnrolledAt: time.Now(),
	}

	if err := s.enrollmentRepo.Create(ctx, enrollment); err != nil {
		return nil, err
	}

	return s.toEnrollmentResponseDTO(enrollment), nil
}

func (s *EnrollmentService) Unenroll(ctx context.Context, userID, courseID uuid.UUID) error {
	enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return err
	}
	if enrollment == nil {
		return errors.New("not enrolled in this course")
	}

	return s.enrollmentRepo.Delete(ctx, enrollment.ID)
}

func (s *EnrollmentService) GetMyEnrollments(ctx context.Context, userID uuid.UUID, page, pageSize int) (*dto.EnrollmentListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	enrollments, total, err := s.enrollmentRepo.GetByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]dto.EnrollmentResponseDTO, len(enrollments))
	for i := range enrollments {
		d := s.toEnrollmentResponseDTO(&enrollments[i])
		d.CourseTitle = enrollments[i].Course.Title
		d.CourseSlug = enrollments[i].Course.Slug
		d.CourseThumbnail = enrollments[i].Course.ThumbnailURL
		if enrollments[i].Course.Category != nil {
			d.CourseCategory = enrollments[i].Course.Category.Name
		}
		result[i] = *d
	}

	return &dto.EnrollmentListResponseDTO{
		Enrollments: result,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

func (s *EnrollmentService) GetEnrollmentDetail(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*dto.EnrollmentDetailDTO, error) {
	enrollment, err := s.enrollmentRepo.GetDetailByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if enrollment == nil {
		return nil, errors.New("enrollment not found")
	}
	if enrollment.UserID != userID {
		return nil, errors.New("enrollment not found")
	}

	detail := &dto.EnrollmentDetailDTO{
		EnrollmentResponseDTO: *s.toEnrollmentResponseDTO(enrollment),
	}
	detail.CourseTitle = enrollment.Course.Title
	detail.CourseThumbnail = enrollment.Course.ThumbnailURL

	progressDTOs := make([]dto.LessonProgressResponseDTO, len(enrollment.LessonProgress))
	for i, lp := range enrollment.LessonProgress {
		progressDTOs[i] = *s.toLessonProgressResponseDTO(&lp)
	}
	detail.LessonProgress = progressDTOs

	return detail, nil
}

func (s *EnrollmentService) UpdateLessonProgress(ctx context.Context, userID, lessonID uuid.UUID, req dto.UpdateLessonProgressDTO) (*dto.LessonProgressResponseDTO, error) {
	lesson, err := s.lessonRepo.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if lesson == nil {
		return nil, errors.New("lesson not found")
	}

	// Find courseID via lesson -> section -> course (single JOIN query)
	courseID, err := s.enrollmentRepo.GetCourseIDByLessonID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	// Verify user is enrolled in this course
	enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	if enrollment == nil {
		return nil, errors.New("not enrolled in the course containing this lesson")
	}

	progress, err := s.enrollmentRepo.GetLessonProgress(ctx, userID, lessonID)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	if progress == nil {
		progress = &model.LessonProgress{
			UserID:         userID,
			LessonID:       lessonID,
			EnrollmentID:   enrollment.ID,
			Status:         "not_started",
			LastAccessedAt: now,
		}
	}

	if req.Status != nil {
		progress.Status = *req.Status
		if *req.Status == "completed" && progress.CompletedAt == nil {
			progress.CompletedAt = &now
		}
	}
	if req.ProgressPercent != nil {
		progress.ProgressPercent = *req.ProgressPercent
	}
	if req.VideoWatchedSecs != nil {
		progress.VideoWatchedSecs = *req.VideoWatchedSecs
	}
	progress.LastAccessedAt = now

	if err := s.enrollmentRepo.UpsertLessonProgress(ctx, progress); err != nil {
		return nil, err
	}

	// Recalculate enrollment progress
	if err := s.recalculateProgress(ctx, enrollment); err != nil {
		return nil, err
	}

	return s.toLessonProgressResponseDTO(progress), nil
}

func (s *EnrollmentService) recalculateProgress(ctx context.Context, enrollment *model.Enrollment) error {
	completed, err := s.enrollmentRepo.CountCompletedMandatory(ctx, enrollment.ID)
	if err != nil {
		return err
	}

	total, err := s.enrollmentRepo.CountTotalMandatory(ctx, enrollment.CourseID)
	if err != nil {
		return err
	}

	var progressPercent decimal.Decimal
	if total > 0 {
		progressPercent = decimal.NewFromInt(completed).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(total))
	}

	enrollment.ProgressPercent = progressPercent
	if err := s.enrollmentRepo.UpdateEnrollmentProgress(ctx, enrollment.ID, progressPercent); err != nil {
		return err
	}

	// Auto-complete enrollment when 100%
	if progressPercent.Equal(decimal.NewFromInt(100)) {
		now := time.Now()
		enrollment.CompletedAt = &now
		if err := s.enrollmentRepo.Update(ctx, enrollment); err != nil {
			return err
		}
	}

	return nil
}

func (s *EnrollmentService) toEnrollmentResponseDTO(enrollment *model.Enrollment) *dto.EnrollmentResponseDTO {
	resp := &dto.EnrollmentResponseDTO{
		ID:              enrollment.ID,
		UserID:          enrollment.UserID,
		CourseID:        enrollment.CourseID,
		EnrolledAt:      enrollment.EnrolledAt.Format("2006-01-02T15:04:05Z"),
		ProgressPercent: enrollment.ProgressPercent,
	}
	if enrollment.CompletedAt != nil {
		formatted := enrollment.CompletedAt.Format("2006-01-02T15:04:05Z")
		resp.CompletedAt = &formatted
	}
	if enrollment.LastAccessedAt != nil {
		formatted := enrollment.LastAccessedAt.Format("2006-01-02T15:04:05Z")
		resp.LastAccessedAt = &formatted
	}
	return resp
}

func (s *EnrollmentService) toLessonProgressResponseDTO(lp *model.LessonProgress) *dto.LessonProgressResponseDTO {
	resp := &dto.LessonProgressResponseDTO{
		ID:               lp.ID,
		UserID:           lp.UserID,
		LessonID:         lp.LessonID,
		EnrollmentID:     lp.EnrollmentID,
		Status:           lp.Status,
		ProgressPercent:  lp.ProgressPercent,
		VideoWatchedSecs: lp.VideoWatchedSecs,
		LastAccessedAt:   lp.LastAccessedAt.Format("2006-01-02T15:04:05Z"),
	}
	if lp.CompletedAt != nil {
		formatted := lp.CompletedAt.Format("2006-01-02T15:04:05Z")
		resp.CompletedAt = &formatted
	}
	return resp
}
