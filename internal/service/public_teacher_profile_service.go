package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ErrTeacherPublicProfileNotFound: id không phải giảng viên đang hoạt động (không tồn tại, không có vai trò
// TEACHER, bị khoá, đã xoá) - handler trả 404, không phân biệt các lý do.
var ErrTeacherPublicProfileNotFound = errors.New("teacher not found")

// PublicTeacherProfileServiceInterface - PG-M2, contract: plans/261013-qa2-role-findings/contract-be3.md §3.
type PublicTeacherProfileServiceInterface interface {
	Get(ctx context.Context, teacherID uuid.UUID) (*dto.PublicTeacherPageDTO, error)
}

type PublicTeacherProfileService struct {
	repo repository.PublicTeacherProfileRepositoryInterface
}

func NewPublicTeacherProfileService(repo repository.PublicTeacherProfileRepositoryInterface) *PublicTeacherProfileService {
	return &PublicTeacherProfileService{repo: repo}
}

func (s *PublicTeacherProfileService) Get(ctx context.Context, teacherID uuid.UUID) (*dto.PublicTeacherPageDTO, error) {
	user, err := s.repo.GetTeacher(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrTeacherPublicProfileNotFound
	}
	visibility, err := s.repo.ProfileVisibility(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	courses, err := s.repo.ListPublishedCourses(ctx, teacherID)
	if err != nil {
		return nil, err
	}

	page := &dto.PublicTeacherPageDTO{
		ID:       user.ID,
		UserName: user.UserName,
		FullName: user.FullName,
		Courses:  make([]dto.PublicTeacherCourseDTO, 0, len(courses)),
	}
	// Review QA2 L4: người xem trang này là khách ẩn danh, nên chỉ `public` được xem đầy đủ; mọi giá trị khác (hidden,
	// private, friends, giá trị lạ - fail-closed) chỉ còn tên + khoá đã xuất bản.
	if visibility != ProfileVisibilityPublic {
		page.ProfileHidden = true
	} else if err := s.fillFullProfile(ctx, page, user); err != nil {
		return nil, err
	}
	for _, c := range courses {
		page.Courses = append(page.Courses, toPublicTeacherCourseDTO(c))
	}
	return page, nil
}

// fillFullProfile: phần chỉ dành cho hồ sơ công khai đầy đủ: avatar, bio, hồ sơ chuyên môn đã duyệt, thống kê, ngày tham gia.
func (s *PublicTeacherProfileService) fillFullProfile(ctx context.Context, page *dto.PublicTeacherPageDTO, user *model.User) error {
	profile, err := s.repo.GetApprovedProfile(ctx, user.ID)
	if err != nil {
		return err
	}
	stats, err := s.repo.PublishedCourseStats(ctx, user.ID)
	if err != nil {
		return err
	}
	page.AvatarURL, page.Bio, page.IsVerified, page.JoinedAt = user.AvatarURL, user.Bio, user.IsVerified, &user.CreatedAt
	page.Stats = &dto.PublicTeacherStatsDTO{
		CourseCount:   stats.CourseCount,
		StudentCount:  stats.StudentCount,
		ReviewCount:   stats.ReviewCount,
		AverageRating: weightedAverageRating(stats.RatingPoints, stats.ReviewCount),
	}
	if profile != nil {
		page.Specialization, page.Education, page.ExperienceYears, page.Department =
			profile.Specialization, profile.Education, profile.ExperienceYears, profile.Department
	}
	return nil
}

// weightedAverageRating = Σ(rating×reviews)/Σreviews, 2 chữ số thập phân; 0 khi chưa có đánh giá nào.
func weightedAverageRating(ratingPoints decimal.Decimal, reviews int64) decimal.Decimal {
	if reviews <= 0 {
		return decimal.Zero
	}
	return ratingPoints.Div(decimal.NewFromInt(reviews)).Round(2)
}

func toPublicTeacherCourseDTO(c model.Course) dto.PublicTeacherCourseDTO {
	return dto.PublicTeacherCourseDTO{
		ID:                c.ID,
		Title:             c.Title,
		Slug:              c.Slug,
		ShortDescription:  c.ShortDescription,
		ThumbnailURL:      c.ThumbnailURL,
		Level:             c.Level,
		Price:             c.Price,
		DiscountPrice:     c.DiscountPrice,
		IsFree:            c.IsFree,
		AverageRating:     c.AverageRating,
		TotalReviews:      c.TotalReviews,
		TotalStudents:     c.TotalStudents,
		TotalLessons:      c.TotalLessons,
		TotalDurationMins: c.TotalDurationMins,
		PublishedAt:       c.PublishedAt,
	}
}
