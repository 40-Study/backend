package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Luồng thí sinh (contract §3.4, §4). Đáp án KHÔNG bao giờ đi ra ở đây, trừ MyResult khi phase
// ENDED/FINALIZED (engine.GetContestAttemptReview).

func isOpenPhase(phase string) bool {
	return phase == model.ContestPhaseUpcoming || phase == model.ContestPhaseActive
}

func isClosedPhase(phase string) bool {
	return phase == model.ContestPhaseEnded || phase == model.ContestPhaseFinalized
}

// loadPublished — người ngoài chỉ thấy cuộc thi PUBLISHED; chủ/admin thấy mọi trạng thái.
func (s *ContestService) loadPublished(c *model.Contest, err error, actor *ContestActor) (*model.Contest, error) {
	if err != nil {
		return nil, err
	}
	if c == nil || (c.Status != model.ContestStatusPublished && !actor.canManage(c)) {
		return nil, ErrContestNotFound
	}
	return c, nil
}

func (s *ContestService) hasEnrollment(ctx context.Context, c *model.Contest, userID uuid.UUID) (bool, error) {
	if c.CourseID == nil {
		return true, nil
	}
	e, err := s.enrollments.GetByUserAndCourse(ctx, userID, *c.CourseID)
	return e != nil, err
}

// joinBlock trả "" khi được đăng ký; thứ tự kiểm giống Join (contract §3.4).
func (s *ContestService) joinBlock(ctx context.Context, c *model.Contest, actor *ContestActor, now time.Time) (string, error) {
	switch {
	case !actor.isStudent():
		return model.ContestJoinBlockRoleNotAllowed, nil
	case actor.isOwnerOf(c):
		return model.ContestJoinBlockOwner, nil
	case c.Status != model.ContestStatusPublished || !isOpenPhase(c.Phase(now)):
		return model.ContestJoinBlockClosed, nil
	}
	ok, err := s.hasEnrollment(ctx, c, actor.UserID)
	if err != nil {
		return "", err
	}
	if !ok {
		return model.ContestJoinBlockCourseRequired, nil
	}
	if c.MaxParticipants > 0 && c.ParticipantCount >= c.MaxParticipants {
		return model.ContestJoinBlockFull, nil
	}
	return "", nil
}

func (s *ContestService) myParticipation(ctx context.Context, c *model.Contest, userID uuid.UUID, now time.Time) (*dto.MyParticipationDTO, *repository.ParticipationRow, error) {
	parts, err := s.repo.ParticipationsOfUser(ctx, userID, []uuid.UUID{c.ID})
	if err != nil {
		return nil, nil, err
	}
	p, ok := parts[c.ID]
	if !ok {
		return nil, nil, nil
	}
	awards, err := s.repo.AwardsOfUser(ctx, userID, []uuid.UUID{c.ID})
	if err != nil {
		return nil, nil, err
	}
	var aw *repository.AwardRow
	if a, ok := awards[c.ID]; ok {
		aw = &a
	}
	return toParticipation(c, p, aw, now), &p, nil
}

// Detail — #7.
func (s *ContestService) Detail(ctx context.Context, slug string, actor *ContestActor) (*dto.ContestDetailDTO, error) {
	c, err := s.repo.GetBySlug(ctx, slug)
	if c, err = s.loadPublished(c, err, actor); err != nil {
		return nil, err
	}
	now := s.now()
	stats, err := s.statsFor(ctx, *c)
	if err != nil {
		return nil, err
	}
	out := &dto.ContestDetailDTO{ContestSummaryDTO: toSummary(c, stats, now), ServerTime: now,
		CertificateMinPercentage: c.CertificateMinPercentage}
	reason := model.ContestJoinBlockClosed
	if actor == nil {
		if isOpenPhase(c.Phase(now)) {
			reason = model.ContestJoinBlockLoginRequired
		}
	} else {
		mp, _, err := s.myParticipation(ctx, c, actor.UserID, now)
		if err != nil {
			return nil, err
		}
		out.Viewer.MyParticipation = mp
		if mp != nil {
			reason = model.ContestJoinBlockAlreadyJoined
		} else if reason, err = s.joinBlock(ctx, c, actor, now); err != nil {
			return nil, err
		}
	}
	out.Viewer.CanJoin = reason == ""
	if reason != "" {
		out.Viewer.JoinBlockReason = &reason
	}
	return out, nil
}

// Join — #12.
func (s *ContestService) Join(ctx context.Context, id uuid.UUID, actor *ContestActor) (*dto.MyParticipationDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil || c.Status != model.ContestStatusPublished {
		return nil, ErrContestNotFound
	}
	now := s.now()
	switch reason, err := s.joinBlock(ctx, c, actor, now); {
	case err != nil:
		return nil, err
	case reason == model.ContestJoinBlockRoleNotAllowed:
		return nil, ErrContestRoleNotAllowed
	case reason == model.ContestJoinBlockOwner:
		return nil, ErrContestOwnerCannotJoin
	case reason == model.ContestJoinBlockClosed:
		return nil, ErrContestClosed
	case reason == model.ContestJoinBlockCourseRequired:
		return nil, ErrContestCourseRequired
	}
	// FULL không chặn ở đây: bộ đếm nguyên tử trong transaction mới là nguồn sự thật (race).
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		n, err := s.repo.InsertParticipantTx(tx, id, actor.UserID)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrContestAlreadyJoined
		}
		if n, err = s.repo.IncrementParticipantCountTx(tx, id); err != nil {
			return err
		}
		if n == 0 {
			return ErrContestFull
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	mp, _, err := s.myParticipation(ctx, c, actor.UserID, now)
	return mp, err
}

// Start — #13, idempotent (contract §4.1).
func (s *ContestService) Start(ctx context.Context, id uuid.UUID, actor *ContestActor) (*dto.ContestStartResponseDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil || c.Status != model.ContestStatusPublished || c.QuizID == nil {
		return nil, ErrContestNotFound
	}
	if !actor.isStudent() {
		return nil, ErrContestRoleNotAllowed
	}
	now := s.now()
	var attemptID uuid.UUID
	var startedAt time.Time
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		p, err := s.repo.LockParticipantTx(tx, id, actor.UserID)
		if err != nil {
			return err
		}
		if p == nil {
			return ErrContestNotJoined
		}
		if p.AttemptID != nil {
			a, err := s.repo.GetAttemptTx(tx, *p.AttemptID)
			if err != nil || a == nil {
				return fmt.Errorf("contest start: load attempt %v: %w", p.AttemptID, err)
			}
			switch attemptStatus(c, &a.ID, &a.StartedAt, a.CompletedAt, now) {
			case model.ContestAttemptSubmitted:
				return ErrContestAlreadySubmitted
			case model.ContestAttemptExpired:
				return ErrContestDeadlinePassed
			}
			attemptID, startedAt = a.ID, a.StartedAt // reload trang: trả lại ĐÚNG attempt cũ
			return nil
		}
		if c.Phase(now) != model.ContestPhaseActive {
			return ErrContestNotActive
		}
		newID, err := s.engine.CreateContestAttemptTx(ctx, tx, *c.QuizID, actor.UserID, now)
		if err != nil {
			return err
		}
		n, err := s.repo.SetParticipantAttemptTx(tx, p.ID, newID)
		if err != nil {
			return err
		}
		if n != 1 { // không thể xảy ra khi đang giữ FOR UPDATE — báo lỗi thay vì tạo attempt mồ côi
			return fmt.Errorf("contest start: participant %s already has an attempt", p.ID)
		}
		attemptID, startedAt = newID, now
		return nil
	})
	if err != nil {
		return nil, err
	}
	questions, err := s.engine.GetContestAttemptQuestions(ctx, *c.QuizID, attemptID)
	if err != nil {
		return nil, err
	}
	return &dto.ContestStartResponseDTO{AttemptID: attemptID, DeadlineAt: contestDeadline(c, startedAt),
		ServerTime: now, DurationMinutes: derefInt(c.DurationMinutes), Questions: questions}, nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// Submit — #14 (contract §4.2). Chấm ở server qua engine; response chỉ có điểm tổng.
func (s *ContestService) Submit(ctx context.Context, id uuid.UUID, actor *ContestActor, req *dto.ContestSubmitRequest) (*dto.ContestSubmitResponseDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil || c.Status != model.ContestStatusPublished || c.QuizID == nil {
		return nil, ErrContestNotFound
	}
	if !actor.isStudent() {
		return nil, ErrContestRoleNotAllowed
	}
	p, err := s.repo.GetParticipant(ctx, id, actor.UserID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrContestNotJoined
	}
	if p.AttemptID == nil || *p.AttemptID != req.AttemptID {
		return nil, ErrContestAttemptMismatch
	}
	a, err := s.repo.GetAttemptTx(s.repo.DB(ctx), req.AttemptID)
	if err != nil || a == nil {
		return nil, fmt.Errorf("contest submit: load attempt: %w", err)
	}
	switch attemptStatus(c, &a.ID, &a.StartedAt, a.CompletedAt, s.now()) {
	case model.ContestAttemptSubmitted:
		return nil, ErrContestAlreadySubmitted
	case model.ContestAttemptExpired:
		return nil, ErrContestDeadlinePassed
	}
	res, err := s.engine.SubmitContestAttempt(ctx, *c.QuizID, actor.UserID, a.ID, req.Answers, derefInt(c.DurationMinutes)*60)
	if errors.Is(err, ErrQuizAttemptAlreadySubmitted) {
		return nil, ErrContestAlreadySubmitted
	}
	if err != nil {
		return nil, err
	}
	return &dto.ContestSubmitResponseDTO{AttemptID: a.ID, Score: res.Score, TotalPoints: res.TotalPoints,
		Percentage: res.Percentage, TimeSpentSeconds: res.TimeSpentSecs, SubmittedAt: res.CompletedAt}, nil
}
