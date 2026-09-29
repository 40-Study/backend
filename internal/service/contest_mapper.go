package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Dựng DTO từ model đã preload (repository.withDetails) + thống kê câu hỏi. Hàm thuần, không
// đọc DB — mọi dữ liệu phụ được nạp theo lô ở contest_query_service.go.

func contestBase(c *model.Contest, stats map[uuid.UUID]repository.QuizStat, now time.Time) dto.ContestBaseDTO {
	b := dto.ContestBaseDTO{
		ID: c.ID, Slug: c.Slug, Title: c.Title, Description: c.Description, BannerURL: c.BannerURL,
		Type: string(c.Type), Status: c.Status, Phase: c.Phase(now),
		StartTime: c.StartTime, EndTime: c.EndTime, MaxParticipants: c.MaxParticipants,
		ParticipantCount: c.ParticipantCount, IsPublic: c.IsPublic, TotalPoints: decimal.Zero,
		CreatorName: displayName(&c.Creator), FinalizedAt: c.FinalizedAt,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if c.DurationMinutes != nil {
		b.DurationMinutes = *c.DurationMinutes
	}
	if c.Course != nil {
		b.Course = &dto.ContestCourseBriefDTO{ID: c.Course.ID, Title: c.Course.Title, Slug: c.Course.Slug}
	}
	if c.QuizID != nil {
		if st, ok := stats[*c.QuizID]; ok {
			b.QuestionCount, b.TotalPoints = st.QuestionCount, st.TotalPoints
		}
	}
	for _, p := range c.Prizes {
		if p.VoucherID != nil {
			b.HasVoucherPrize = true
		}
	}
	return b
}

// toSummary — dạng CÔNG KHAI: voucher chỉ {id, name}, KHÔNG có mã (contract §2.1, test (d)).
func toSummary(c *model.Contest, stats map[uuid.UUID]repository.QuizStat, now time.Time) dto.ContestSummaryDTO {
	out := dto.ContestSummaryDTO{ContestBaseDTO: contestBase(c, stats, now), Prizes: []dto.ContestPrizeDTO{}}
	for _, p := range c.Prizes {
		item := dto.ContestPrizeDTO{ID: p.ID, RankFrom: p.RankFrom, RankTo: p.RankTo, GrantCertificate: p.GrantCertificate}
		if p.Voucher != nil {
			item.Voucher = &dto.ContestVoucherBriefDTO{ID: p.Voucher.ID, Name: p.Voucher.Name}
		}
		out.Prizes = append(out.Prizes, item)
	}
	return out
}

// toManage — dạng quản lý (chủ/admin): voucher có mã, kèm thông tin duyệt.
func toManage(c *model.Contest, stats map[uuid.UUID]repository.QuizStat, now time.Time) dto.ContestManageDTO {
	out := dto.ContestManageDTO{
		ContestBaseDTO: contestBase(c, stats, now), Prizes: []dto.ContestPrizeAdminDTO{},
		CourseID: c.CourseID, CertificateMinPercentage: c.CertificateMinPercentage,
		SubmittedAt: c.SubmittedAt, ReviewedAt: c.ReviewedAt, RejectReason: c.RejectReason,
		CancelReason: c.CancelReason, CreatedBy: c.CreatedBy, CreatorEmail: c.Creator.Email,
	}
	for _, p := range c.Prizes {
		item := dto.ContestPrizeAdminDTO{ID: p.ID, RankFrom: p.RankFrom, RankTo: p.RankTo, GrantCertificate: p.GrantCertificate}
		if p.Voucher != nil {
			item.Voucher = &dto.ContestVoucherAdminDTO{ID: p.Voucher.ID, Code: p.Voucher.Code, Name: p.Voucher.Name}
		}
		out.Prizes = append(out.Prizes, item)
	}
	if c.Quiz != nil {
		out.Quiz = &dto.ContestQuizBriefDTO{ID: c.Quiz.ID, Title: c.Quiz.Title, QuestionCount: out.QuestionCount}
	}
	return out
}

// contestDeadline = min(started_at + duration, end_time) (contract §2.1).
func contestDeadline(c *model.Contest, startedAt time.Time) time.Time {
	d := 0
	if c.DurationMinutes != nil {
		d = *c.DurationMinutes
	}
	dl := startedAt.Add(time.Duration(d) * time.Minute)
	if c.EndTime.Before(dl) {
		return c.EndTime
	}
	return dl
}

// attemptStatus theo contract §2.1: chưa có attempt → NOT_STARTED; đã nộp → SUBMITTED; quá
// deadline + 30s chưa nộp → EXPIRED; còn lại IN_PROGRESS.
func attemptStatus(c *model.Contest, attemptID *uuid.UUID, startedAt, completedAt *time.Time, now time.Time) string {
	switch {
	case attemptID == nil || startedAt == nil:
		return model.ContestAttemptNotStarted
	case completedAt != nil:
		return model.ContestAttemptSubmitted
	case now.After(contestDeadline(c, *startedAt).Add(model.ContestSubmitGraceSeconds * time.Second)):
		return model.ContestAttemptExpired
	}
	return model.ContestAttemptInProgress
}

func toParticipation(c *model.Contest, p repository.ParticipationRow, award *repository.AwardRow, now time.Time) *dto.MyParticipationDTO {
	out := &dto.MyParticipationDTO{
		JoinedAt: p.JoinedAt, AttemptID: p.AttemptID, StartedAt: p.StartedAt, SubmittedAt: p.CompletedAt,
		AttemptStatus: attemptStatus(c, p.AttemptID, p.StartedAt, p.CompletedAt, now), Rank: p.Rank,
	}
	if p.StartedAt != nil {
		dl := contestDeadline(c, *p.StartedAt)
		out.DeadlineAt = &dl
	}
	// Điểm chỉ hiện khi đã nộp (attempt đang làm có score NULL; không để lộ trạng thái chấm dở).
	if p.CompletedAt != nil {
		out.Score, out.TotalPoints, out.Percentage, out.TimeSpentSeconds = p.Score, p.TotalPoints, p.Percentage, p.TimeSpentSeconds
	}
	if award != nil {
		out.Award = &dto.ContestAwardBriefDTO{CertificateNumber: award.CertificateNumber}
		if award.VoucherID != nil {
			out.Award.Voucher = &dto.ContestVoucherAdminDTO{ID: *award.VoucherID, Code: deref(award.VoucherCode), Name: deref(award.VoucherName)}
		}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func quizIDsOf(cs []model.Contest) []uuid.UUID {
	var ids []uuid.UUID
	for _, c := range cs {
		if c.QuizID != nil {
			ids = append(ids, *c.QuizID)
		}
	}
	return ids
}

func contestIDsOf(cs []model.Contest) []uuid.UUID {
	ids := make([]uuid.UUID, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return ids
}

func (s *ContestService) statsFor(ctx context.Context, cs ...model.Contest) (map[uuid.UUID]repository.QuizStat, error) {
	return s.repo.QuizStats(ctx, quizIDsOf(cs))
}
