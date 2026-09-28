package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// Finalize — chốt kết quả (contract §5 + ĐÍNH CHÍNH 28/09: CHỈ admin, route
// POST /admin/contests/:id/finalize). Một transaction, idempotent:
//   - FOR UPDATE trên dòng contests tuần tự hoá 2 lần bấm đồng thời; lần sau thấy finalized_at
//     khác NULL và chỉ đếm lại, không phát thêm, không gửi thông báo lần 2.
//   - Phát thưởng (chứng nhận/voucher) qua issuer.IssueAwardTx CÙNG tx: một voucher hỏng làm
//     rollback toàn bộ, không để nửa số thí sinh có giải còn nửa kia không.
//   - Thông báo gửi SAU commit; lỗi thông báo chỉ log, phản ánh ở notified_count.
func (s *ContestService) Finalize(ctx context.Context, id uuid.UUID, actor *ContestActor) (*dto.FinalizeResultDTO, error) {
	if actor == nil || !actor.IsAdmin {
		return nil, ErrContestForbidden
	}
	var result dto.FinalizeResultDTO
	var notices []ContestResultNotice
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		c, err := s.repo.LockByIDTx(tx, id)
		if err != nil {
			return err
		}
		if c == nil {
			return ErrContestNotFound
		}
		if c.FinalizedAt != nil {
			counts, err := s.repo.AwardCountsTx(tx, id)
			result = dto.FinalizeResultDTO{ContestID: id, FinalizedAt: *c.FinalizedAt, AlreadyFinalized: true,
				RankedCount: counts.RankedCount, AwardCount: counts.AwardCount,
				CertificateCount: counts.CertificateCount, VoucherCount: counts.VoucherCount}
			return err
		}
		if c.Status != model.ContestStatusPublished {
			return ErrContestInvalidStatus
		}
		now := s.now()
		if now.Before(c.EndTime.Add(model.ContestFinalizeDelaySeconds * time.Second)) {
			return ErrContestNotEnded
		}
		notices, err = s.rankAndAward(ctx, tx, c, now, &result)
		if err != nil {
			return err
		}
		n, err := s.repo.UpdateStatusTx(tx, id, []string{model.ContestStatusPublished}, "finalized_at IS NULL",
			map[string]interface{}{"finalized_at": now, "finalized_by": actor.UserID})
		if err != nil {
			return err
		}
		if n != 1 { // row lock ở trên phải chặn được — tới đây là lỗi hệ thống, không phải nghiệp vụ
			return fmt.Errorf("contest finalize: finalized_at already set for %s under row lock", id)
		}
		result.ContestID, result.FinalizedAt = id, now
		return nil
	})
	// Voucher giải không phát được: vẫn 409 CONTEST_VOUCHER_UNAVAILABLE, nhưng nói rõ người thắng và
	// voucher nào (B2 VoucherGrantError) để admin sửa giải rồi chốt lại.
	var ge *VoucherGrantError
	if errors.As(err, &ge) {
		return nil, ErrContestVoucherUnusable.withDetails(ge.Error(), &dto.ContestVoucherGrantErrorDTO{
			UserID: ge.UserID, UserName: ge.UserName, Rank: ge.Rank,
			VoucherID: ge.VoucherID, VoucherCode: ge.VoucherCode, Reason: ge.Reason,
		})
	}
	if errors.Is(err, ErrVoucherUnavailableForGrant) {
		return nil, ErrContestVoucherUnusable
	}
	if err != nil {
		return nil, err
	}
	if !result.AlreadyFinalized && len(notices) > 0 {
		sent, nerr := s.issuer.NotifyContestResults(ctx, notices)
		if nerr != nil {
			log.Printf("[WARN] contest %s: notify results sent=%d/%d: %v", id, sent, len(notices), nerr)
		}
		result.NotifiedCount = sent
	}
	return &result, nil
}

func prizeForRank(prizes []model.ContestPrize, rank int) *model.ContestPrize {
	for i := range prizes {
		if prizes[i].RankFrom <= rank && rank <= prizes[i].RankTo {
			return &prizes[i]
		}
	}
	return nil
}

// rankAndAward — bước 4–5 của §5: ghi hạng, phát giải trong khoảng hạng hoặc chứng nhận theo ngưỡng.
func (s *ContestService) rankAndAward(ctx context.Context, tx *gorm.DB, c *model.Contest, now time.Time,
	result *dto.FinalizeResultDTO) ([]ContestResultNotice, error) {
	prizes, err := s.repo.PrizesTx(tx, c.ID)
	if err != nil {
		return nil, err
	}
	rows, _, err := s.repo.RankedRows(tx, c.ID, false, 0, 0)
	if err != nil {
		return nil, err
	}
	notices := make([]ContestResultNotice, 0, len(rows))
	for _, r := range rows {
		rank := r.Rank
		if err := s.repo.SetRankTx(tx, c.ID, r.UserID, rank); err != nil {
			return nil, err
		}
		result.RankedCount++
		notice := ContestResultNotice{UserID: r.UserID, ContestID: c.ID, ContestTitle: c.Title, ContestSlug: c.Slug, Rank: &rank}
		grant, ok := buildGrant(c, prizeForRank(prizes, rank), r, now)
		if ok {
			issued, err := s.issuer.IssueAwardTx(ctx, tx, grant)
			if err != nil {
				return nil, err
			}
			award := model.ContestAward{ID: uuid.New(), ContestID: c.ID, UserID: r.UserID, Rank: grant.Rank, IssuedAt: now}
			if issued != nil {
				award.CertificateNumber, award.UserVoucherID = issued.CertificateNumber, issued.UserVoucherID
			}
			if err := s.repo.InsertAwardTx(tx, &award); err != nil {
				return nil, err
			}
			result.AwardCount++
			if award.CertificateNumber != nil {
				result.CertificateCount++
				notice.HasCertificate = true
			}
			if award.UserVoucherID != nil {
				result.VoucherCount++
				notice.HasVoucher = true
			}
		}
		notices = append(notices, notice)
	}
	return notices, nil
}

// buildGrant: trong khoảng giải → chứng nhận/voucher theo giải (rank = hạng thật); ngoài top mà
// đạt certificate_min_percentage → chỉ chứng nhận, rank NULL (contract §5 bước 5).
func buildGrant(c *model.Contest, prize *model.ContestPrize, r repository.RankedRow, now time.Time) (ContestAwardGrant, bool) {
	g := ContestAwardGrant{ContestID: c.ID, UserID: r.UserID, ContestTitle: c.Title, IssuedAt: now}
	if prize != nil {
		rank := r.Rank
		g.Rank, g.GrantCertificate, g.VoucherID = &rank, prize.GrantCertificate, prize.VoucherID
	}
	if min := c.CertificateMinPercentage; min != nil && r.Percentage.GreaterThanOrEqual(*min) {
		g.GrantCertificate = true
	}
	return g, g.GrantCertificate || g.VoucherID != nil
}
