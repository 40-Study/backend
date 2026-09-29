package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// ContestValidationError — lỗi dữ liệu đầu vào ngoài phạm vi tag validate (vd phần trăm 0..100);
// handler trả shape {"message":"Validation failed","errors":[...]} như utils.ValidateStruct.
type ContestValidationError struct{ Errors []utils.ValidationError }

func (e *ContestValidationError) Error() string { return "validation failed: " + e.Errors[0].Message }

func invalidField(field, msg string) *ContestValidationError {
	return &ContestValidationError{Errors: []utils.ValidationError{{Field: field, Tag: "invalid", Message: msg}}}
}

// validateSchedule — contract §2.1: end_time > start_time + duration, start_time > now.
func validateSchedule(start, end time.Time, durationMinutes int, now time.Time) error {
	if !start.After(now) || !end.After(start.Add(time.Duration(durationMinutes)*time.Minute)) {
		return ErrContestInvalidSchedule
	}
	return nil
}

// validatePrizes — contract §1.4: ≤10 dòng, 1 ≤ rank_from ≤ rank_to ≤ 100, không chồng khoảng,
// mỗi dòng phải có chứng nhận hoặc voucher. Giảng viên (không phải admin) không được gắn voucher.
func validatePrizes(prizes []dto.ContestPrizeInput, isAdmin bool) error {
	if len(prizes) > model.ContestMaxPrizes {
		return ErrContestPrizesInvalid
	}
	for _, p := range prizes {
		if p.VoucherID != nil && !isAdmin {
			return ErrContestVoucherAdminOnly
		}
	}
	sorted := append([]dto.ContestPrizeInput(nil), prizes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].RankFrom < sorted[j].RankFrom })
	for i, p := range sorted {
		if p.RankFrom < 1 || p.RankTo < p.RankFrom || p.RankTo > model.ContestMaxPrizeRank {
			return ErrContestPrizesInvalid
		}
		if !p.GrantCertificate && p.VoucherID == nil {
			return ErrContestPrizesInvalid
		}
		if i > 0 && p.RankFrom <= sorted[i-1].RankTo {
			return ErrContestPrizesInvalid
		}
	}
	return nil
}

func prizeModels(in []dto.ContestPrizeInput) []model.ContestPrize {
	out := make([]model.ContestPrize, 0, len(in))
	for _, p := range in {
		out = append(out, model.ContestPrize{ID: uuid.New(), RankFrom: p.RankFrom, RankTo: p.RankTo,
			GrantCertificate: p.GrantCertificate, VoucherID: p.VoucherID})
	}
	return out
}

// checkVouchers — voucher của giải phải còn phát được (lúc tạo/sửa bởi admin, duyệt, sửa giải).
func (s *ContestService) checkVouchers(ctx context.Context, voucherIDs []uuid.UUID) error {
	bad, err := s.repo.UnusableVoucherIDs(ctx, voucherIDs, s.now())
	if err != nil {
		return err
	}
	if len(bad) > 0 {
		return ErrContestVoucherUnusable
	}
	return nil
}

func inputVoucherIDs(prizes []dto.ContestPrizeInput) []uuid.UUID {
	var ids []uuid.UUID
	for _, p := range prizes {
		if p.VoucherID != nil {
			ids = append(ids, *p.VoucherID)
		}
	}
	return ids
}

func storedVoucherIDs(prizes []model.ContestPrize) []uuid.UUID {
	var ids []uuid.UUID
	for _, p := range prizes {
		if p.VoucherID != nil {
			ids = append(ids, *p.VoucherID)
		}
	}
	return ids
}

// checkQuiz — contract §3.3. ownerID = người tạo cuộc thi; anyOwner = true khi admin thao tác trên
// cuộc thi CỦA CHÍNH admin (admin được chọn mọi quiz standalone). selfID = cuộc thi đang sửa
// (quiz đã gắn chính nó thì không tính là "đã dùng").
func (s *ContestService) checkQuiz(ctx context.Context, quizID, ownerID uuid.UUID, anyOwner bool, selfID *uuid.UUID) error {
	info, err := s.repo.GetQuizInfo(ctx, quizID)
	if err != nil {
		return err
	}
	if info == nil || info.LessonID != nil || info.CourseID != nil || info.SessionID != nil {
		return ErrContestQuizInvalid
	}
	if !anyOwner && (info.CreatedBy == nil || *info.CreatedBy != ownerID) {
		return ErrContestQuizInvalid
	}
	if info.QuestionCount == 0 || info.IneligibleCount > 0 || info.AttemptCount > 0 {
		return ErrContestQuizInvalid
	}
	if info.ContestID != nil && (selfID == nil || *info.ContestID != *selfID) {
		return ErrContestQuizInUse
	}
	return nil
}

// validateUpsert gom mọi kiểm tra của tạo/sửa theo thứ tự rẻ → đắt.
func (s *ContestService) validateUpsert(ctx context.Context, actor *ContestActor, req *dto.ContestUpsertRequest,
	ownerID uuid.UUID, selfID *uuid.UUID) error {
	if p := req.CertificateMinPercentage; p != nil && (p.LessThan(decimal.Zero) || p.GreaterThan(decimal.NewFromInt(100))) {
		return invalidField("certificate_min_percentage", "certificate_min_percentage must be between 0 and 100")
	}
	if err := validateSchedule(req.StartTime, req.EndTime, req.DurationMinutes, s.now()); err != nil {
		return err
	}
	if err := validatePrizes(req.Prizes, actor.IsAdmin); err != nil {
		return err
	}
	if req.CourseID != nil {
		ok, err := s.repo.CourseExists(ctx, *req.CourseID)
		if err != nil {
			return err
		}
		if !ok {
			return invalidField("course_id", "course_id does not exist")
		}
	}
	anyOwner := actor.IsAdmin && actor.UserID == ownerID
	if err := s.checkQuiz(ctx, req.QuizID, ownerID, anyOwner, selfID); err != nil {
		return err
	}
	return s.checkVouchers(ctx, inputVoucherIDs(req.Prizes))
}

// isDuplicateKey — unique idx_contests_quiz_id bị vi phạm khi 2 request cùng gắn 1 quiz (race).
func isDuplicateKey(err error) bool {
	return err != nil && (errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "23505"))
}
