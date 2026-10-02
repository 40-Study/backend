package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type LeaderboardServiceInterface interface {
	// viewer = người xem (nil = khách): quyết định ai thấy tên thật của người đặt leaderboard_display ẩn danh/username.
	GetLeaderboard(ctx context.Context, periodType string, limit int, viewer *LeaderboardViewer) (*dto.LeaderboardResponse, error)
	GetMyRank(ctx context.Context, userID uuid.UUID, periodType string) (*dto.MyRankResponse, error)
}

type LeaderboardService struct {
	repo *repository.LeaderboardRepository
}

func NewLeaderboardService(repo *repository.LeaderboardRepository) *LeaderboardService {
	return &LeaderboardService{repo: repo}
}

func (s *LeaderboardService) GetLeaderboard(ctx context.Context, periodType string, limit int, viewer *LeaderboardViewer) (*dto.LeaderboardResponse, error) {
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
		entries[i] = leaderboardEntry(r, viewer)
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
		// Người xem chính là chủ dòng: luôn thấy tên thật của mình, bất kể cài đặt.
		entry := leaderboardEntry(*row, &LeaderboardViewer{UserID: userID})
		resp.Entry = &entry
	}
	return resp, nil
}

// leaderboardEntry dựng một dòng trả ra cho người xem, đã áp cài đặt riêng tư của chủ dòng.
func leaderboardEntry(r repository.LeaderboardRow, viewer *LeaderboardViewer) dto.LeaderboardEntryDTO {
	p := presentLeaderboardUser(r.LeaderboardDisplay, r.UserID, r.FullName, r.UserName, r.AvatarURL, viewer)
	return dto.LeaderboardEntryDTO{
		Rank:        r.Rank,
		UserID:      p.UserID,
		UserName:    p.UserName,
		FullName:    p.FullName,
		AvatarURL:   p.AvatarURL,
		DisplayName: p.DisplayName,
		Points:      r.Points,
		IsMe:        p.IsMe,
	}
}

// ErrInvalidLeaderboardPeriod — kỳ xếp hạng không thuộc model.IsValidLeaderboardPeriodType.
var ErrInvalidLeaderboardPeriod = errors.New("invalid period: must be weekly, monthly, or all_time")

func validatePeriodType(p string) error {
	if !model.IsValidLeaderboardPeriodType(p) {
		return ErrInvalidLeaderboardPeriod
	}
	return nil
}
