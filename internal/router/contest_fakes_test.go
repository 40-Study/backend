package router

// Test double cho ranh giới lane B2 (contract §6). Engine là QuizService THẬT của B2 (đề/chấm/review
// thật — để test rò đáp án bắt được cả fill_blank), chỉ bọc thêm độ trễ khi tạo attempt cho test
// race. Issuer là fake: ghi user_vouchers BẰNG tx được truyền vào (kiểm rollback khi voucher hỏng)
// và phát hiện thông báo gửi trước commit.

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

// delayEngine bọc engine thật; createDelay giữ tx start mở lâu hơn (test race): không có FOR UPDATE
// thì request thứ hai chắc chắn đọc thấy attempt_id NULL trong cửa sổ này và tạo attempt thứ hai.
type delayEngine struct {
	service.ContestQuizEngine
	createDelay time.Duration
}

func (d *delayEngine) CreateContestAttemptTx(ctx context.Context, tx *gorm.DB, quizID, userID uuid.UUID, startedAt time.Time) (uuid.UUID, error) {
	time.Sleep(d.createDelay)
	return d.ContestQuizEngine.CreateContestAttemptTx(ctx, tx, quizID, userID, startedAt)
}
type fakeIssuer struct {
	mu               sync.Mutex
	notified         [][]service.ContestResultNotice
	db               *gorm.DB // đọc ngoài tx để kiểm "thông báo chỉ sau commit"
	uncommittedCalls int
	// delegate (tuỳ chọn): phát thưởng bằng ContestRewardService THẬT của B2 (lỗi voucher chi tiết),
	// thông báo vẫn qua fake để giữ kiểm tra "sau commit".
	delegate *service.ContestRewardService
}

func (f *fakeIssuer) IssueAwardTx(ctx context.Context, tx *gorm.DB, g service.ContestAwardGrant) (*service.ContestAwardIssued, error) {
	if f.delegate != nil {
		return f.delegate.IssueAwardTx(ctx, tx, g)
	}
	out := &service.ContestAwardIssued{}
	if g.VoucherID != nil {
		var n int64
		if err := tx.Model(&model.Voucher{}).Where("id = ? AND is_active = true", *g.VoucherID).Count(&n).Error; err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, service.ErrVoucherUnavailableForGrant
		}
		uv := model.UserVoucher{ID: uuid.New(), UserID: g.UserID, VoucherID: *g.VoucherID, Source: "contest_reward", SavedAt: time.Now()}
		if err := tx.Create(&uv).Error; err != nil {
			return nil, err
		}
		out.UserVoucherID = &uv.ID
	}
	if g.GrantCertificate {
		num := "CONTEST-" + g.IssuedAt.Format("20060102") + "-" + uuid.NewString()[:8]
		out.CertificateNumber = &num
	}
	return out, nil
}

func (f *fakeIssuer) NotifyContestResults(_ context.Context, notices []service.ContestResultNotice) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Kết nối pool NGOÀI tx chốt: nếu thông báo được gửi trong tx (chưa commit) thì kết nối này
	// chưa thấy finalized_at → ghi nhận vi phạm contract §5 bước 7.
	if f.db != nil && len(notices) > 0 {
		var committed int64
		f.db.Table("contests").Where("id = ? AND finalized_at IS NOT NULL", notices[0].ContestID).Count(&committed)
		if committed == 0 {
			f.uncommittedCalls++
		}
	}
	f.notified = append(f.notified, notices)
	return len(notices), nil
}

func (f *fakeIssuer) notifyCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.notified)
}
