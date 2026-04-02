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

	return &dto.PublicProfileResponse{
		UserID:    row.UserID,
		UserName:  row.UserName,
		FullName:  row.FullName,
		AvatarURL: row.AvatarURL,
		Bio:       row.Bio,
		Stats: dto.UserStatsDTO{
			TotalPoints:      row.TotalPoints,
			Level:            row.Level,
			LevelProgress:    row.LevelProgress,
			CurrentStreak:    row.CurrentStreak,
			LongestStreak:    row.LongestStreak,
			TotalCheckins:    row.TotalCheckins,
			AchievementCount: row.AchievementCount,
		},
	}, nil
}
