package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Phase 3 duyệt khoá học (2026-09-28). Quyết định #3: CHỈ SYSTEM_ADMIN duyệt (permission
// COURSES_APPROVE_ALL, gate ở router) — không dùng COURSES_APPROVE_OWN_ORG.

var (
	ErrReviewReasonRequired = errors.New("reason is required")
	ErrReviewReasonTooLong  = errors.New("reason must be at most 1000 characters")
	ErrInvalidReviewFilter  = errors.New("invalid status filter")
)

const maxReviewReasonLength = 1000

// NormalizeReviewReason trim + kiểm bắt buộc/độ dài — dùng chung cho từ chối khoá học và từ chối
// hồ sơ giáo viên (quyết định #5: từ chối PHẢI ghi lý do).
func NormalizeReviewReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	if reason == "" {
		return "", ErrReviewReasonRequired
	}
	if utf8.RuneCountInString(reason) > maxReviewReasonLength {
		return "", ErrReviewReasonTooLong
	}
	return reason, nil
}

func isOneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}

type CourseReviewServiceInterface interface {
	SubmitForReview(ctx context.Context, courseID, actorID uuid.UUID) (*dto.CourseReviewResultDTO, error)
	// WithdrawReview (Q5, QA vòng 2): giảng viên chủ khoá rút yêu cầu duyệt, khoá về draft để sửa.
	WithdrawReview(ctx context.Context, courseID, actorID uuid.UUID) (*dto.CourseReviewResultDTO, error)
	ListForReview(ctx context.Context, status, keyword string, page, pageSize int) (*dto.AdminCourseReviewListDTO, error)
	Approve(ctx context.Context, courseID, adminID uuid.UUID) (*dto.CourseReviewResultDTO, error)
	Reject(ctx context.Context, courseID, adminID uuid.UUID, rawReason string) (*dto.CourseReviewResultDTO, error)
}

type CourseReviewService struct {
	repo repository.CourseReviewRepositoryInterface
	// mapper: tái dùng CourseService.toCourseResponseDTO (một nguồn map DTO duy nhất) — hàm này
	// không đọc field nào của receiver nên zero-value là đủ.
	mapper *CourseService
}

func NewCourseReviewService(repo repository.CourseReviewRepositoryInterface) *CourseReviewService {
	return &CourseReviewService{repo: repo, mapper: &CourseService{}}
}

func toReviewResult(c *model.Course) *dto.CourseReviewResultDTO {
	out := &dto.CourseReviewResultDTO{ID: c.ID, Status: c.Status}
	if c.SubmittedAt != nil {
		s := c.SubmittedAt.Format(time.RFC3339)
		out.SubmittedAt = &s
	}
	return out
}

func (s *CourseReviewService) SubmitForReview(ctx context.Context, courseID, actorID uuid.UUID) (*dto.CourseReviewResultDTO, error) {
	course, err := s.repo.ApplyReviewAction(ctx, courseID, repository.CourseActionSubmit, &actorID, nil, nil)
	if err != nil {
		return nil, err
	}
	return toReviewResult(course), nil
}

// WithdrawReview — pending_review -> draft, chỉ chủ khoá (ownerID). Khoá dòng course giống nộp
// duyệt: admin duyệt/từ chối cùng lúc thì một trong hai bên nhận ErrCourseInvalidReviewStatus.
func (s *CourseReviewService) WithdrawReview(ctx context.Context, courseID, actorID uuid.UUID) (*dto.CourseReviewResultDTO, error) {
	course, err := s.repo.ApplyReviewAction(ctx, courseID, repository.CourseActionWithdraw, &actorID, nil, nil)
	if err != nil {
		return nil, err
	}
	return toReviewResult(course), nil
}

func (s *CourseReviewService) ListForReview(ctx context.Context, status, keyword string, page, pageSize int) (*dto.AdminCourseReviewListDTO, error) {
	if status == "" {
		status = model.CourseStatusPendingReview
	}
	if !isOneOf(status, model.CourseStatuses) {
		return nil, ErrInvalidReviewFilter
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	courses, total, err := s.repo.ListForReview(ctx, repository.AdminCourseReviewFilter{
		Status: status, Keyword: keyword, Page: page, PageSize: pageSize,
	})
	if err != nil {
		return nil, err
	}
	items := make([]dto.AdminCourseReviewItemDTO, len(courses))
	for i := range courses {
		c := &courses[i]
		name := c.Instructor.UserName
		if c.Instructor.FullName != nil && *c.Instructor.FullName != "" {
			name = *c.Instructor.FullName
		}
		items[i] = dto.AdminCourseReviewItemDTO{
			CourseResponseDTO: *s.mapper.toCourseResponseDTO(c),
			InstructorName:    name,
			InstructorEmail:   c.Instructor.Email,
		}
	}
	return &dto.AdminCourseReviewListDTO{Courses: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *CourseReviewService) Approve(ctx context.Context, courseID, adminID uuid.UUID) (*dto.CourseReviewResultDTO, error) {
	course, err := s.repo.ApplyReviewAction(ctx, courseID, repository.CourseActionApprove, nil, &adminID, nil)
	if err != nil {
		return nil, err
	}
	return toReviewResult(course), nil
}

func (s *CourseReviewService) Reject(ctx context.Context, courseID, adminID uuid.UUID, rawReason string) (*dto.CourseReviewResultDTO, error) {
	reason, err := NormalizeReviewReason(rawReason)
	if err != nil {
		return nil, err
	}
	course, err := s.repo.ApplyReviewAction(ctx, courseID, repository.CourseActionReject, nil, &adminID, &reason)
	if err != nil {
		return nil, err
	}
	return toReviewResult(course), nil
}
