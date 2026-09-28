package service

// contest_reward_service.go — ContestRewardService hiện thực ContestRewardIssuer (contract "Cuộc
// thi" §5, §6): phát chứng nhận + voucher trong transaction chốt kết quả, và gửi thông báo SAU khi
// transaction đó đã commit.

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
)

var _ ContestRewardIssuer = (*ContestRewardService)(nil)

// rewardVoucherGranter / rewardNotificationSender: interface hẹp để test thay được, và để
// *VoucherService / *NotificationService (kiểu cụ thể trong app.Services) truyền thẳng vào.
type rewardVoucherGranter interface {
	GrantVoucherTx(ctx context.Context, tx *gorm.DB, userID, voucherID uuid.UUID, source, notes string) (*model.UserVoucher, error)
}

type rewardNotificationSender interface {
	SendNotification(req dto.CreateNotificationDTO) error
}

type ContestRewardService struct {
	vouchers      rewardVoucherGranter
	notifications rewardNotificationSender
}

func NewContestRewardService(vouchers rewardVoucherGranter, notifications rewardNotificationSender) *ContestRewardService {
	return &ContestRewardService{vouchers: vouchers, notifications: notifications}
}

// rewardCertificateNumber: CONTEST-YYYYMMDD-xxxxxxxx, cùng khuôn CERT- của certificate_service.go.
// Trùng số (8 ký tự hex) bị unique idx_contest_awards_certificate_number chặn và làm rollback cả
// lần chốt — xác suất rất nhỏ, và chốt lại là an toàn vì chốt idempotent.
func rewardCertificateNumber(issuedAt time.Time) string {
	return fmt.Sprintf("CONTEST-%s-%s", issuedAt.Format("20060102"), uuid.New().String()[:8])
}

// IssueAwardTx phát phần thưởng của MỘT người TRÊN tx của caller. Không ghi contest_awards — việc
// đó là của ContestService (B1), dùng số chứng nhận/user_voucher_id trả về đây. Voucher hết hiệu
// lực trả ErrVoucherUnavailableForGrant để caller rollback toàn bộ lần chốt.
func (s *ContestRewardService) IssueAwardTx(ctx context.Context, tx *gorm.DB, g ContestAwardGrant) (*ContestAwardIssued, error) {
	if tx == nil {
		return nil, errors.New("IssueAwardTx requires a transaction")
	}
	if !g.GrantCertificate && g.VoucherID == nil {
		return nil, errors.New("contest award grants nothing")
	}
	issuedAt := g.IssuedAt
	if issuedAt.IsZero() {
		issuedAt = time.Now()
	}

	issued := &ContestAwardIssued{}
	if g.VoucherID != nil {
		notes := "Giải cuộc thi " + g.ContestTitle
		if g.Rank != nil {
			notes = fmt.Sprintf("%s - hạng %d", notes, *g.Rank)
		}
		uv, err := s.vouchers.GrantVoucherTx(ctx, tx, g.UserID, *g.VoucherID, model.UserVoucherSourceContestReward, notes)
		if err != nil {
			return nil, err
		}
		issued.UserVoucherID = &uv.ID
	}
	if g.GrantCertificate {
		number := rewardCertificateNumber(issuedAt)
		issued.CertificateNumber = &number
	}
	return issued, nil
}

// NotifyContestResults gửi mỗi người một thông báo. Chỉ được gọi SAU khi transaction chốt đã commit
// (§5 bước 7) nên không nhận tx: thông báo không thể "rò" ra ngoài khi lần chốt bị rollback, và một
// lần gửi lỗi cũng không kéo lùi kết quả đã chốt. Lỗi từng người được log và gộp vào err; sent là số
// thông báo gửi thành công (ContestService trả nó ở notified_count).
func (s *ContestRewardService) NotifyContestResults(ctx context.Context, notices []ContestResultNotice) (int, error) {
	sent := 0
	var errs []error
	for _, n := range notices {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		refType := "contest"
		refID := n.ContestID
		err := s.notifications.SendNotification(dto.CreateNotificationDTO{
			Title:            "Kết quả cuộc thi " + n.ContestTitle,
			Content:          rewardNoticeContent(n),
			NotificationType: "achievement",
			ReferenceType:    &refType,
			ReferenceID:      &refID,
			UserIDs:          []uuid.UUID{n.UserID},
		})
		if err != nil {
			log.Printf("[contest] gửi thông báo kết quả contest=%s user=%s lỗi: %v", n.ContestID, n.UserID, err)
			errs = append(errs, err)
			continue
		}
		sent++
	}
	return sent, errors.Join(errs...)
}

// rewardNoticeContent: nội dung thông báo theo phần thưởng thực nhận.
func rewardNoticeContent(n ContestResultNotice) string {
	head := fmt.Sprintf("Cuộc thi \"%s\" đã có kết quả.", n.ContestTitle)
	if n.Rank != nil {
		head = fmt.Sprintf("Bạn xếp hạng %d trong cuộc thi \"%s\".", *n.Rank, n.ContestTitle)
	}
	switch {
	case n.HasCertificate && n.HasVoucher:
		return head + " Bạn nhận được chứng nhận và voucher, xem voucher trong ví của bạn."
	case n.HasCertificate:
		return head + " Bạn nhận được chứng nhận."
	case n.HasVoucher:
		return head + " Bạn nhận được voucher, xem trong ví của bạn."
	default:
		return head + " Cảm ơn bạn đã tham gia."
	}
}
