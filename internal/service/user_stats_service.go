package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

type UserStatsServiceInterface interface {
	// viewerID nil = khách chưa đăng nhập; viewerIsAdmin = người xem có SYSTEM_SETTINGS_MANAGE.
	GetPublicProfile(ctx context.Context, userID uuid.UUID, viewerID *uuid.UUID, viewerIsAdmin bool) (*dto.PublicProfileResponse, error)
}

type UserStatsService struct {
	repo     *repository.UserStatsRepository
	prefRepo repository.UserPreferenceRepositoryInterface
}

func NewUserStatsService(repo *repository.UserStatsRepository, prefRepo repository.UserPreferenceRepositoryInterface) *UserStatsService {
	return &UserStatsService{repo: repo, prefRepo: prefRepo}
}

// Giá trị cài đặt "Hiển thị hồ sơ" (user_preferences.profile_visibility).
const (
	ProfileVisibilityPublic  = "public"  // mọi người xem đầy đủ
	ProfileVisibilityFriends = "friends" // chưa có tính năng bạn bè: xử lý như riêng tư
	ProfileVisibilityPrivate = "private" // người khác chỉ thấy tên và avatar
	ProfileVisibilityHidden  = "hidden"  // ẩn hẳn: người khác nhận 404
)

// ErrPublicProfileNotFound: không có người dùng, HOẶC người đó ẩn hồ sơ hẳn với người xem này (404 cho cả
// hai để không lộ việc tài khoản có tồn tại). Handler nhận diện qua message "user not found".
var ErrPublicProfileNotFound = errors.New("user not found")

// profileVisibility đọc cài đặt riêng tư của chủ hồ sơ; chưa có dòng cài đặt = công khai (mặc định của
// cột). Giá trị lạ trong DB xử lý như riêng tư (fail-closed) ở GetPublicProfile.
func (s *UserStatsService) profileVisibility(userID uuid.UUID) (string, error) {
	pref, err := s.prefRepo.GetByUserID(userID)
	if err != nil {
		return "", err
	}
	if pref == nil || pref.ProfileVisibility == "" {
		return ProfileVisibilityPublic, nil
	}
	return pref.ProfileVisibility, nil
}

// GetPublicProfile (S6): trước đây bỏ qua hoàn toàn cài đặt riêng tư, ai (kể cả khách) cũng thấy đầy đủ
// hồ sơ, thành tích, hoạt động, khoá đã hoàn thành. Nay: chủ hồ sơ và admin luôn thấy đầy đủ; người khác
// theo cài đặt của chủ hồ sơ (public đầy đủ; private/friends chỉ tên + avatar; hidden 404).
func (s *UserStatsService) GetPublicProfile(ctx context.Context, userID uuid.UUID, viewerID *uuid.UUID, viewerIsAdmin bool) (*dto.PublicProfileResponse, error) {
	row, err := s.repo.GetPublicProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrPublicProfileNotFound
	}

	isSelf := viewerID != nil && *viewerID == userID
	if !isSelf && !viewerIsAdmin {
		visibility, err := s.profileVisibility(userID)
		if err != nil {
			return nil, err
		}
		switch visibility {
		case ProfileVisibilityPublic:
			// đầy đủ, đi tiếp
		case ProfileVisibilityHidden:
			return nil, ErrPublicProfileNotFound
		default:
			// private, friends (chưa có bạn bè để xác minh) và mọi giá trị không nhận ra: chỉ tên + avatar.
			return &dto.PublicProfileResponse{
				UserID:               row.UserID,
				UserName:             row.UserName,
				FullName:             row.FullName,
				AvatarURL:            row.AvatarURL,
				IsPrivate:            true,
				FeaturedAchievements: []dto.PublicProfileAchievementDTO{},
				Activity:             []dto.PublicProfileActivityDTO{},
				CompletedCourses:     []dto.PublicProfileCompletedCourseDTO{},
			}, nil
		}
	}

	achievements, err := s.repo.GetPublicProfileAchievements(ctx, userID, 12)
	if err != nil {
		return nil, err
	}

	activity, err := s.repo.GetPublicProfileActivity(ctx, userID, 365)
	if err != nil {
		return nil, err
	}

	completedCourses, err := s.repo.GetPublicProfileCompletedCourses(ctx, userID, 12)
	if err != nil {
		return nil, err
	}

	achievementDTOs := make([]dto.PublicProfileAchievementDTO, 0, len(achievements))
	for _, achievement := range achievements {
		achievementDTOs = append(achievementDTOs, dto.PublicProfileAchievementDTO{
			ID:       achievement.ID,
			Name:     achievement.Name,
			IconURL:  achievement.IconURL,
			BadgeURL: achievement.BadgeURL,
			Category: achievement.Category,
			EarnedAt: achievement.EarnedAt,
		})
	}

	activityDTOs := make([]dto.PublicProfileActivityDTO, 0, len(activity))
	for _, item := range activity {
		activityDTOs = append(activityDTOs, dto.PublicProfileActivityDTO{
			Date:  item.Date.Format("2006-01-02"),
			Count: item.Count,
		})
	}

	completedCourseDTOs := make([]dto.PublicProfileCompletedCourseDTO, 0, len(completedCourses))
	for _, course := range completedCourses {
		completedCourseDTOs = append(completedCourseDTOs, dto.PublicProfileCompletedCourseDTO{
			ID:           course.ID,
			Title:        course.Title,
			ThumbnailURL: course.ThumbnailURL,
			CompletedAt:  course.CompletedAt,
		})
	}

	return &dto.PublicProfileResponse{
		UserID:               row.UserID,
		UserName:             row.UserName,
		FullName:             row.FullName,
		AvatarURL:            row.AvatarURL,
		Bio:                  row.Bio,
		JoinedAt:             row.JoinedAt,
		FeaturedAchievements: achievementDTOs,
		Activity:             activityDTOs,
		CompletedCourses:     completedCourseDTOs,
		Stats: dto.UserStatsDTO{
			TotalPoints:           row.TotalPoints,
			Level:                 row.Level,
			LevelProgress:         row.LevelProgress,
			CurrentStreak:         row.CurrentStreak,
			LongestStreak:         row.LongestStreak,
			TotalCheckins:         row.TotalCheckins,
			AchievementCount:      row.AchievementCount,
			CoursesCompleted:      row.CoursesCompleted,
			LessonsCompleted:      row.LessonsCompleted,
			TotalStudyTimeMinutes: row.TotalStudyTimeMinutes,
		},
	}, nil
}
