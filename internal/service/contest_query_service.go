package service

import (
	"context"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ContestListQuery — tham số đã được handler kiểm hợp lệ (status ∈ ContestStatuses, phase ∈ ContestPhases).
type ContestListQuery struct {
	Status, Phase, Q string
	CourseID         *uuid.UUID
	Page, Limit      int
}

func (s *ContestService) listContests(ctx context.Context, f repository.ContestListFilter, q ContestListQuery) ([]model.Contest, int64, int, int, error) {
	page, limit := normalizePage(q.Page, q.Limit)
	f.Status, f.Phase, f.Q, f.CourseID, f.Page, f.Limit, f.Now = q.Status, q.Phase, q.Q, q.CourseID, page, limit, s.now()
	items, total, err := s.repo.List(ctx, f)
	return items, total, page, limit, err
}

// ListPublic — #1: chỉ PUBLISHED + is_public (bản nháp/chờ duyệt/bị từ chối/huỷ không bao giờ lộ).
func (s *ContestService) ListPublic(ctx context.Context, q ContestListQuery) (*dto.ContestPage[dto.ContestSummaryDTO], error) {
	items, total, page, limit, err := s.listContests(ctx, repository.ContestListFilter{PublicOnly: true, OrderStartDesc: true}, q)
	if err != nil {
		return nil, err
	}
	stats, err := s.statsFor(ctx, items...)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := &dto.ContestPage[dto.ContestSummaryDTO]{Items: []dto.ContestSummaryDTO{}, TotalCount: total, Page: page, Limit: limit, TotalPages: totalPages(total, limit)}
	for i := range items {
		out.Items = append(out.Items, toSummary(&items[i], stats, now))
	}
	return out, nil
}

// ListMine — #2: cuộc thi mình đã đăng ký, kèm my_participation.
func (s *ContestService) ListMine(ctx context.Context, actor *ContestActor, page, limit int) (*dto.ContestPage[dto.ContestMySummaryDTO], error) {
	items, total, page, limit, err := s.listContests(ctx, repository.ContestListFilter{JoinedBy: &actor.UserID, OrderStartDesc: true},
		ContestListQuery{Page: page, Limit: limit})
	if err != nil {
		return nil, err
	}
	stats, err := s.statsFor(ctx, items...)
	if err != nil {
		return nil, err
	}
	ids := contestIDsOf(items)
	parts, err := s.repo.ParticipationsOfUser(ctx, actor.UserID, ids)
	if err != nil {
		return nil, err
	}
	awards, err := s.repo.AwardsOfUser(ctx, actor.UserID, ids)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := &dto.ContestPage[dto.ContestMySummaryDTO]{Items: []dto.ContestMySummaryDTO{}, TotalCount: total, Page: page, Limit: limit, TotalPages: totalPages(total, limit)}
	for i := range items {
		c := &items[i]
		row := dto.ContestMySummaryDTO{ContestSummaryDTO: toSummary(c, stats, now)}
		if p, ok := parts[c.ID]; ok {
			var aw *repository.AwardRow
			if a, ok := awards[c.ID]; ok {
				aw = &a
			}
			row.MyParticipation = toParticipation(c, p, aw, now)
		}
		out.Items = append(out.Items, row)
	}
	return out, nil
}

func (s *ContestService) manageList(ctx context.Context, f repository.ContestListFilter, q ContestListQuery) (*dto.ContestPage[dto.ContestManageDTO], error) {
	items, total, page, limit, err := s.listContests(ctx, f, q)
	if err != nil {
		return nil, err
	}
	stats, err := s.statsFor(ctx, items...)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := &dto.ContestPage[dto.ContestManageDTO]{Items: []dto.ContestManageDTO{}, TotalCount: total, Page: page, Limit: limit, TotalPages: totalPages(total, limit)}
	for i := range items {
		out.Items = append(out.Items, toManage(&items[i], stats, now))
	}
	return out, nil
}

// ListManage — #3: chỉ cuộc thi DO CHÍNH MÌNH tạo (admin cũng vậy; admin xem mọi cuộc thi ở #19).
func (s *ContestService) ListManage(ctx context.Context, actor *ContestActor, q ContestListQuery) (*dto.ContestPage[dto.ContestManageDTO], error) {
	return s.manageList(ctx, repository.ContestListFilter{CreatedBy: &actor.UserID}, q)
}

// ListAdmin — #19: mọi cuộc thi trừ bản nháp của người khác (bản nháp là việc riêng của GV).
func (s *ContestService) ListAdmin(ctx context.Context, actor *ContestActor, q ContestListQuery) (*dto.ContestPage[dto.ContestManageDTO], error) {
	return s.manageList(ctx, repository.ContestListFilter{AdminViewer: &actor.UserID}, q)
}

// QuizOptions — #4.
func (s *ContestService) QuizOptions(ctx context.Context, actor *ContestActor) ([]dto.ContestQuizOptionDTO, error) {
	var owner *uuid.UUID
	if !actor.IsAdmin {
		owner = &actor.UserID
	}
	rows, err := s.repo.ListEligibleQuizzes(ctx, owner)
	if err != nil {
		return nil, err
	}
	out := []dto.ContestQuizOptionDTO{}
	for _, r := range rows {
		out = append(out, dto.ContestQuizOptionDTO{ID: r.ID, Title: r.Title, QuestionCount: r.QuestionCount, TotalPoints: r.TotalPoints})
	}
	return out, nil
}

// GetManage — #5: người không phải chủ/admin nhận 404 (không lộ sự tồn tại).
func (s *ContestService) GetManage(ctx context.Context, id uuid.UUID, actor *ContestActor) (*dto.ContestManageDTO, error) {
	c, err := s.loadForManage(ctx, id, actor, true)
	if err != nil {
		return nil, err
	}
	return s.manageDTO(ctx, c)
}

func (s *ContestService) manageDTO(ctx context.Context, c *model.Contest) (*dto.ContestManageDTO, error) {
	stats, err := s.statsFor(ctx, *c)
	if err != nil {
		return nil, err
	}
	out := toManage(c, stats, s.now())
	return &out, nil
}

// reloadManage đọc lại sau khi ghi để trả đúng trạng thái đã lưu.
func (s *ContestService) reloadManage(ctx context.Context, id uuid.UUID) (*dto.ContestManageDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrContestNotFound
	}
	return s.manageDTO(ctx, c)
}

// Participants — #6: chủ/admin, mọi phase.
func (s *ContestService) Participants(ctx context.Context, id uuid.UUID, actor *ContestActor, page, limit int) (*dto.ContestPage[dto.ContestParticipantRowDTO], error) {
	c, err := s.loadForManage(ctx, id, actor, true)
	if err != nil {
		return nil, err
	}
	page, limit = normalizePage(page, limit)
	rows, total, err := s.repo.ListParticipants(ctx, c.ID, page, limit)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := &dto.ContestPage[dto.ContestParticipantRowDTO]{Items: []dto.ContestParticipantRowDTO{}, TotalCount: total, Page: page, Limit: limit, TotalPages: totalPages(total, limit)}
	for _, p := range rows {
		mp := toParticipation(c, p, nil, now)
		out.Items = append(out.Items, dto.ContestParticipantRowDTO{
			UserID: p.UserID, UserName: p.UserName, AvatarURL: p.AvatarURL, JoinedAt: p.JoinedAt,
			AttemptStatus: mp.AttemptStatus, Score: mp.Score, Percentage: mp.Percentage,
			TimeSpentSeconds: mp.TimeSpentSeconds, SubmittedAt: mp.SubmittedAt, Rank: p.Rank,
		})
	}
	return out, nil
}
