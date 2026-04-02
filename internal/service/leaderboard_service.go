package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

type LeaderboardServiceInterface interface {
	GetLeaderboard(ctx context.Context, periodType string, limit int) (*dto.LeaderboardResponse, error)
	GetMyRank(ctx context.Context, userID uuid.UUID, periodType string) (*dto.MyRankResponse, error)
}

type LeaderboardService struct {
	repo *repository.LeaderboardRepository
}

func NewLeaderboardService(repo *repository.LeaderboardRepository) *LeaderboardService {
	return &LeaderboardService{repo: repo}
}

func (s *LeaderboardService) GetLeaderboard(ctx context.Context, periodType string, limit int) (*dto.LeaderboardResponse, error) {
	if err := validatePeriodType(periodType); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	period := repository.CurrentPeriod(periodType)
	rows, err := s.repo.GetTopUsers(ctx, periodType, period, limit)
	if err != nil {
		return nil, err
	}

	entries := make([]dto.LeaderboardEntryDTO, len(rows))
	for i, r := range rows {
		entries[i] = dto.LeaderboardEntryDTO{
			Rank:      r.Rank,
			UserID:    r.UserID,
			UserName:  r.UserName,
			FullName:  r.FullName,
			AvatarURL: r.AvatarURL,
			Points:    r.Points,
		}
	}

	return &dto.LeaderboardResponse{
		PeriodType: periodType,
		Period:     period,
		Entries:    entries,
		Total:      len(entries),
	}, nil
}

func (s *LeaderboardService) GetMyRank(ctx context.Context, userID uuid.UUID, periodType string) (*dto.MyRankResponse, error) {
	if err := validatePeriodType(periodType); err != nil {
		return nil, err
	}

	period := repository.CurrentPeriod(periodType)
	row, err := s.repo.GetUserRank(ctx, userID, periodType, period)
	if err != nil {
		return nil, err
	}

	resp := &dto.MyRankResponse{
		PeriodType: periodType,
		Period:     period,
	}
	if row != nil {
		entry := dto.LeaderboardEntryDTO{
			Rank:      row.Rank,
			UserID:    row.UserID,
			UserName:  row.UserName,
			FullName:  row.FullName,
			AvatarURL: row.AvatarURL,
			Points:    row.Points,
		}
		resp.Entry = &entry
	}
	return resp, nil
}

func validatePeriodType(p string) error {
	switch p {
	case "weekly", "monthly", "all_time":
		return nil
	default:
		return errors.New("invalid period: must be weekly, monthly, or all_time")
	}
}
