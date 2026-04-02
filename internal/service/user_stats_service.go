package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

type UserStatsServiceInterface interface {
	GetPublicProfile(ctx context.Context, userID uuid.UUID) (*dto.PublicProfileResponse, error)
}

type UserStatsService struct {
	repo *repository.UserStatsRepository
}

func NewUserStatsService(repo *repository.UserStatsRepository) *UserStatsService {
	return &UserStatsService{repo: repo}
}

func (s *UserStatsService) GetPublicProfile(ctx context.Context, userID uuid.UUID) (*dto.PublicProfileResponse, error) {
	row, err := s.repo.GetPublicProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("user not found")
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
