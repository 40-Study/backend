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
	// classID != nil (B-20): bảng xếp hạng riêng của lớp; chỉ thành viên lớp hoặc admin xem được.
	GetLeaderboard(ctx context.Context, periodType string, limit int, viewer *LeaderboardViewer, classID *uuid.UUID) (*dto.LeaderboardResponse, error)
	GetMyRank(ctx context.Context, userID uuid.UUID, periodType string) (*dto.MyRankResponse, error)
	// ListMyClassBoards: các lớp mà người dùng mở được bảng xếp hạng riêng (?class_id=), để giao diện hiện ô chọn lớp.
	ListMyClassBoards(ctx context.Context, userID uuid.UUID) ([]dto.LeaderboardClassDTO, error)
}

type LeaderboardService struct {
	repo *repository.LeaderboardRepository
}

func NewLeaderboardService(repo *repository.LeaderboardRepository) *LeaderboardService {
	return &LeaderboardService{repo: repo}
}

func (s *LeaderboardService) GetLeaderboard(ctx context.Context, periodType string, limit int, viewer *LeaderboardViewer, classID *uuid.UUID) (*dto.LeaderboardResponse, error) {
	if err := validatePeriodType(periodType); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if classID != nil {
		// Route công khai: nếu không kiểm, khách chỉ cần đoán class_id là liệt kê được học viên của lớp.
		// Khách và người ngoài lớp đều nhận 404 như mọi tài nguyên lớp không xem được.
		if viewer == nil {
			return nil, ErrLeaderboardClassNotFound
		}
		if !viewer.IsAdmin {
			ok, err := s.repo.CanViewClassBoard(ctx, viewer.UserID, *classID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, ErrLeaderboardClassNotFound
			}
		}
	}

	period := repository.CurrentPeriod(periodType)
	rows, err := s.repo.GetTopUsers(ctx, periodType, period, limit, classID)
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

// ListMyClassBoards trả các lớp người dùng là học viên đang ghi danh hoặc giảng viên (cùng luật với việc mở
// bảng theo lớp ở GetLeaderboard); không có lớp nào thì trả mảng rỗng, không phải null.
func (s *LeaderboardService) ListMyClassBoards(ctx context.Context, userID uuid.UUID) ([]dto.LeaderboardClassDTO, error) {
	rows, err := s.repo.ListClassBoardsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.LeaderboardClassDTO, len(rows))
	for i, r := range rows {
		out[i] = dto.LeaderboardClassDTO{ID: r.ID, Name: r.Name}
	}
	return out, nil
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

// ErrLeaderboardClassNotFound — lớp không tồn tại hoặc người xem không có quan hệ với lớp (404, không lộ lớp có hay không).
var ErrLeaderboardClassNotFound = errors.New("class not found")

// ErrInvalidLeaderboardPeriod — kỳ xếp hạng không thuộc model.IsValidLeaderboardPeriodType.
var ErrInvalidLeaderboardPeriod = errors.New("invalid period: must be weekly, monthly, or all_time")

func validatePeriodType(p string) error {
	if !model.IsValidLeaderboardPeriodType(p) {
		return ErrInvalidLeaderboardPeriod
	}
	return nil
}
