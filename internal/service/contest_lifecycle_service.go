package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// Vòng đời cuộc thi (contract §3.1): tạo → DRAFT; sửa/xoá khi DRAFT/REJECTED; gửi duyệt; admin
// duyệt/từ chối/huỷ/sửa giải. Mọi chuyển trạng thái dùng UPDATE ... WHERE status IN (...) và
// RowsAffected, nên 2 request đồng thời không thể cùng thắng.

var editableStatuses = []string{model.ContestStatusDraft, model.ContestStatusRejected}

func contestSlug(title string) string {
	base := utils.GenerateSlug(title)
	if base == "" {
		base = "cuoc-thi"
	}
	if len(base) > 200 {
		base = strings.Trim(base[:200], "-")
	}
	return base + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
}

func upsertValues(req *dto.ContestUpsertRequest) map[string]interface{} {
	d := req.DurationMinutes
	return map[string]interface{}{
		"title": strings.TrimSpace(req.Title), "description": req.Description, "banner_url": req.BannerURL,
		"quiz_id": req.QuizID, "course_id": req.CourseID, "start_time": req.StartTime, "end_time": req.EndTime,
		"duration_minutes": &d, "max_participants": req.MaxParticipants, "is_public": req.IsPublic,
		"certificate_min_percentage": req.CertificateMinPercentage,
	}
}

// transitionErr: UPDATE không đổi dòng nào — cuộc thi đã biến mất (404) hay sai trạng thái (409).
func (s *ContestService) transitionErr(ctx context.Context, id uuid.UUID) error {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if c == nil {
		return ErrContestNotFound
	}
	return ErrContestInvalidStatus
}

// Create — #8.
func (s *ContestService) Create(ctx context.Context, actor *ContestActor, req *dto.ContestUpsertRequest) (*dto.ContestManageDTO, error) {
	if err := s.validateUpsert(ctx, actor, req, actor.UserID, nil); err != nil {
		return nil, err
	}
	d := req.DurationMinutes
	c := model.Contest{
		Title: strings.TrimSpace(req.Title), Slug: contestSlug(req.Title), Description: req.Description,
		BannerURL: req.BannerURL, Type: model.ContestTypeQuiz, Status: model.ContestStatusDraft,
		StartTime: req.StartTime, EndTime: req.EndTime, DurationMinutes: &d, MaxParticipants: req.MaxParticipants,
		IsPublic: req.IsPublic, CreatedBy: actor.UserID, QuizID: &req.QuizID, CourseID: req.CourseID,
		CertificateMinPercentage: req.CertificateMinPercentage,
	}
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error { return s.repo.CreateTx(tx, &c, prizeModels(req.Prizes)) })
	if isDuplicateKey(err) {
		return nil, ErrContestQuizInUse
	}
	if err != nil {
		return nil, err
	}
	return s.reloadManage(ctx, c.ID)
}

// Update — #9: thay toàn bộ nội dung + giải khi DRAFT/REJECTED.
func (s *ContestService) Update(ctx context.Context, id uuid.UUID, actor *ContestActor, req *dto.ContestUpsertRequest) (*dto.ContestManageDTO, error) {
	c, err := s.loadForManage(ctx, id, actor, false)
	if err != nil {
		return nil, err
	}
	if !isOneOf(c.Status, editableStatuses) {
		return nil, ErrContestInvalidStatus
	}
	if err := s.validateUpsert(ctx, actor, req, c.CreatedBy, &c.ID); err != nil {
		return nil, err
	}
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		n, err := s.repo.UpdateStatusTx(tx, id, editableStatuses, "", upsertValues(req))
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrContestInvalidStatus
		}
		return s.repo.ReplacePrizesTx(tx, id, prizeModels(req.Prizes))
	})
	if isDuplicateKey(err) {
		return nil, ErrContestQuizInUse
	}
	if err != nil {
		return nil, err
	}
	return s.reloadManage(ctx, id)
}

// Delete — #10: xoá hẳn để giải phóng quiz.
func (s *ContestService) Delete(ctx context.Context, id uuid.UUID, actor *ContestActor) error {
	if _, err := s.loadForManage(ctx, id, actor, false); err != nil {
		return err
	}
	n, err := s.repo.HardDelete(ctx, id, editableStatuses)
	if err != nil {
		return err
	}
	if n == 0 {
		return s.transitionErr(ctx, id)
	}
	return nil
}

// SubmitReview — #11: chỉ người tạo.
func (s *ContestService) SubmitReview(ctx context.Context, id uuid.UUID, actor *ContestActor) (*dto.ContestManageDTO, error) {
	c, err := s.loadForManage(ctx, id, actor, false)
	if err != nil {
		return nil, err
	}
	if !actor.isOwnerOf(c) {
		return nil, ErrContestForbidden
	}
	if !isOneOf(c.Status, editableStatuses) {
		return nil, ErrContestInvalidStatus
	}
	if err := s.checkReadyForReview(ctx, c, actor.IsAdmin); err != nil {
		return nil, err
	}
	return s.transition(ctx, id, editableStatuses, "", map[string]interface{}{
		"status": model.ContestStatusPendingReview, "submitted_at": s.now(),
	}, nil)
}

// checkReadyForReview — điều kiện chung của gửi duyệt và duyệt: chưa tới giờ thi, quiz còn hợp lệ.
func (s *ContestService) checkReadyForReview(ctx context.Context, c *model.Contest, anyOwner bool) error {
	if !c.StartTime.After(s.now()) {
		return ErrContestStartPassed
	}
	if c.QuizID == nil {
		return ErrContestQuizInvalid
	}
	return s.checkQuiz(ctx, *c.QuizID, c.CreatedBy, anyOwner, &c.ID)
}

// transition chạy UPDATE có điều kiện (+ thay giải nếu prizes != nil) rồi trả bản quản lý mới.
func (s *ContestService) transition(ctx context.Context, id uuid.UUID, from []string, extra string,
	values map[string]interface{}, prizes []model.ContestPrize) (*dto.ContestManageDTO, error) {
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		n, err := s.repo.UpdateStatusTx(tx, id, from, extra, values)
		if err != nil {
			return err
		}
		if n == 0 {
			return errNoRows
		}
		if prizes != nil {
			return s.repo.ReplacePrizesTx(tx, id, prizes)
		}
		return nil
	})
	if err == errNoRows {
		return nil, s.transitionErr(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	return s.reloadManage(ctx, id)
}

var errNoRows = &ContestError{Status: 409, Code: "CONTEST_INVALID_STATUS", Message: "no rows"}

func (s *ContestService) loadForAdmin(ctx context.Context, id uuid.UUID, actor *ContestActor) (*model.Contest, error) {
	if actor == nil || !actor.IsAdmin {
		return nil, ErrContestForbidden
	}
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrContestNotFound
	}
	return c, nil
}

// Approve — #20. prizes nil = giữ giải hiện có (vẫn kiểm voucher còn dùng được).
func (s *ContestService) Approve(ctx context.Context, id uuid.UUID, actor *ContestActor, prizes []dto.ContestPrizeInput) (*dto.ContestManageDTO, error) {
	c, err := s.loadForAdmin(ctx, id, actor)
	if err != nil {
		return nil, err
	}
	from := []string{model.ContestStatusPendingReview}
	if actor.isOwnerOf(c) {
		from = append(from, editableStatuses...)
	}
	if !isOneOf(c.Status, from) {
		return nil, ErrContestInvalidStatus
	}
	if err := s.checkReadyForReview(ctx, c, actor.isOwnerOf(c)); err != nil {
		return nil, err
	}
	var newPrizes []model.ContestPrize
	voucherIDs := storedVoucherIDs(c.Prizes)
	if prizes != nil {
		if err := validatePrizes(prizes, true); err != nil {
			return nil, err
		}
		newPrizes, voucherIDs = prizeModels(prizes), inputVoucherIDs(prizes)
	}
	if err := s.checkVouchers(ctx, voucherIDs); err != nil {
		return nil, err
	}
	now := s.now()
	return s.transition(ctx, id, from, "", map[string]interface{}{
		"status": model.ContestStatusPublished, "reviewed_by": actor.UserID, "reviewed_at": now, "reject_reason": nil,
	}, newPrizes)
}

// Reject — #21.
func (s *ContestService) Reject(ctx context.Context, id uuid.UUID, actor *ContestActor, rawReason string) (*dto.ContestManageDTO, error) {
	reason, err := normalizeContestReason(rawReason)
	if err != nil {
		return nil, err
	}
	if _, err := s.loadForAdmin(ctx, id, actor); err != nil {
		return nil, err
	}
	return s.transition(ctx, id, []string{model.ContestStatusPendingReview}, "", map[string]interface{}{
		"status": model.ContestStatusRejected, "reject_reason": reason, "reviewed_by": actor.UserID, "reviewed_at": s.now(),
	}, nil)
}

// Cancel — #22: chờ duyệt, hoặc đã công bố nhưng CHƯA chốt.
func (s *ContestService) Cancel(ctx context.Context, id uuid.UUID, actor *ContestActor, rawReason string) (*dto.ContestManageDTO, error) {
	reason, err := normalizeContestReason(rawReason)
	if err != nil {
		return nil, err
	}
	if _, err := s.loadForAdmin(ctx, id, actor); err != nil {
		return nil, err
	}
	return s.transition(ctx, id, []string{model.ContestStatusPendingReview, model.ContestStatusPublished},
		"finalized_at IS NULL", map[string]interface{}{"status": model.ContestStatusCancelled, "cancel_reason": reason}, nil)
}

// UpdatePrizes — #23: PUBLISHED/PENDING_REVIEW chưa chốt. UPDATE contests (khoá dòng) trước khi
// thay giải nên không chen được vào giữa một lần chốt đang chạy (chốt giữ FOR UPDATE).
func (s *ContestService) UpdatePrizes(ctx context.Context, id uuid.UUID, actor *ContestActor, prizes []dto.ContestPrizeInput) (*dto.ContestManageDTO, error) {
	if _, err := s.loadForAdmin(ctx, id, actor); err != nil {
		return nil, err
	}
	if err := validatePrizes(prizes, true); err != nil {
		return nil, err
	}
	if err := s.checkVouchers(ctx, inputVoucherIDs(prizes)); err != nil {
		return nil, err
	}
	// Cùng quy ước với Approve: body {} (prizes nil) giữ nguyên giải; mảng rỗng tường minh xoá hết.
	var newPrizes []model.ContestPrize
	if prizes != nil {
		newPrizes = prizeModels(prizes)
	}
	return s.transition(ctx, id, []string{model.ContestStatusPublished, model.ContestStatusPendingReview},
		"finalized_at IS NULL", map[string]interface{}{"updated_at": s.now()}, newPrizes)
}
