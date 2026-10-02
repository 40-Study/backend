package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// answersOpen: cuộc thi đã đóng VÀ đã qua ân hạn nộp bài (model.Contest.AnswersAvailableAt).
// Dùng chung cho đáp án trong my-result và bảng xếp hạng công khai.
func answersOpen(c *model.Contest, now time.Time) bool {
	return isClosedPhase(c.Phase(now)) && !now.Before(c.AnswersAvailableAt())
}

// MyResult — #15: chỉ bài của CHÍNH người gọi. Đáp án (questions) chỉ trả từ AnswersAvailableAt
// (end_time + ân hạn) — trước đó null để thí sinh đã nộp không chuyển đáp án cho người còn nộp được.
func (s *ContestService) MyResult(ctx context.Context, id uuid.UUID, actor *ContestActor) (*dto.ContestMyResultDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrContestNotFound
	}
	now := s.now()
	mp, p, err := s.myParticipation(ctx, c, actor.UserID, now)
	if err != nil {
		return nil, err
	}
	if p == nil || p.CompletedAt == nil || p.AttemptID == nil {
		return nil, ErrContestResultNotFound
	}
	out := &dto.ContestMyResultDTO{MyParticipation: mp, AnswersAvailableAt: c.AnswersAvailableAt()}
	if answersOpen(c, now) {
		if out.Questions, err = s.engine.GetContestAttemptReview(ctx, *p.AttemptID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Leaderboard — #16: công khai từ AnswersAvailableAt (cùng mốc với đáp án); chủ/admin xem mọi lúc. Trước khi chốt xếp hạng
// sống theo §4.4, sau khi chốt đọc contest_participants.rank đã ghi.
func (s *ContestService) Leaderboard(ctx context.Context, id uuid.UUID, actor *ContestActor, page, limit int) (*dto.LeaderboardPageDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if c, err = s.loadPublished(c, err, actor); err != nil {
		return nil, err
	}
	if !answersOpen(c, s.now()) && !actor.canManage(c) {
		return nil, ErrContestLeaderboardHidden
	}
	page, limit = normalizePage(page, limit)
	finalized := c.FinalizedAt != nil
	rows, total, err := s.repo.RankedRows(s.repo.DB(ctx), c.ID, finalized, page, limit)
	if err != nil {
		return nil, err
	}
	// Bảng xếp hạng công khai: thí sinh đặt leaderboard_display = anonymous hiện là "Học viên ẩn danh" (không avatar)
	// với mọi người trừ chính họ và admin; "username" chỉ hiện tên đăng nhập. Tên thật vẫn nằm ở route quản trị.
	var viewer *LeaderboardViewer
	if actor != nil {
		viewer = &LeaderboardViewer{UserID: actor.UserID, IsAdmin: actor.IsAdmin}
	}
	out := &dto.LeaderboardPageDTO{Finalized: finalized}
	out.Items, out.TotalCount, out.Page, out.Limit, out.TotalPages = contestLeaderboardItems(rows, viewer), total, page, limit, totalPages(total, limit)
	return out, nil
}

// contestLeaderboardItems dựng các dòng BXH cuộc thi cho người xem, đã áp leaderboard_display của từng thí sinh.
// Luôn trả slice không nil (JSON `[]`, không `null`).
func contestLeaderboardItems(rows []repository.RankedRow, viewer *LeaderboardViewer) []dto.LeaderboardItemDTO {
	items := make([]dto.LeaderboardItemDTO, 0, len(rows))
	for _, r := range rows {
		who := presentLeaderboardUser(r.LeaderboardDisplay, r.UserID, r.FullName, r.LoginName, r.AvatarURL, viewer)
		items = append(items, dto.LeaderboardItemDTO{
			Rank: r.Rank, UserName: who.DisplayName, AvatarURL: who.AvatarURL, Score: r.Score, TotalPoints: r.TotalPoints,
			Percentage: r.Percentage, TimeSpentSeconds: r.TimeSpentSeconds, SubmittedAt: r.CompletedAt,
			IsMe: who.IsMe,
		})
	}
	return items
}

// Certificate — #17: chứng nhận của CHÍNH người gọi.
func (s *ContestService) Certificate(ctx context.Context, id uuid.UUID, actor *ContestActor) (*dto.ContestCertificateDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrContestNotFound
	}
	awards, err := s.repo.AwardsOfUser(ctx, actor.UserID, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	a, ok := awards[id]
	if !ok || a.CertificateNumber == nil {
		return nil, ErrContestCertNotFound
	}
	parts, err := s.repo.ParticipationsOfUser(ctx, actor.UserID, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	return &dto.ContestCertificateDTO{CertificateNumber: *a.CertificateNumber, UserName: parts[id].UserName,
		ContestTitle: c.Title, ContestSlug: c.Slug, Rank: a.Rank, IssuedAt: a.IssuedAt}, nil
}
