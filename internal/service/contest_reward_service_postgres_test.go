package service

// Lane B2 "Cuộc thi" — GrantVoucherTx + ContestRewardService trên Postgres thật (contract §5, §9 B2 (e)).

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func newTestVoucherService(f *contestFixture) *VoucherService {
	return NewVoucherService(repository.NewVoucherRepository(f.db), repository.NewUserRepository(f.db))
}

func (f *contestFixture) voucher(active bool, endDate *time.Time) uuid.UUID {
	f.t.Helper()
	v := model.Voucher{Code: "QA-CONTEST-" + uuid.NewString()[:8], Name: "Giải cuộc thi QA",
		DiscountUnit: model.DiscountUnitMoney, DiscountMethod: model.DiscountMethodFixed, IsActive: true, EndDate: endDate}
	if err := f.db.Create(&v).Error; err != nil {
		f.t.Fatalf("tạo voucher: %v", err)
	}
	if !active { // is_active có default:true ở DB — GORM bỏ qua false khi Create.
		if err := f.db.Model(&model.Voucher{}).Where("id = ?", v.ID).Update("is_active", false).Error; err != nil {
			f.t.Fatalf("tắt voucher: %v", err)
		}
	}
	return v.ID
}

// (e) GrantVoucherTx chạy TRÊN tx của caller: rollback không để lại user_vouchers; commit ghi đúng
// source contest_reward.
func TestGrantVoucherTx_RollbackSach_CommitGhiDungNguon(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	user := f.user("student")
	voucherID := f.voucher(true, nil)
	ctx := context.Background()

	rollback := errors.New("rollback có chủ đích")
	err := f.db.Transaction(func(tx *gorm.DB) error {
		if _, err := vs.GrantVoucherTx(ctx, tx, user, voucherID, model.UserVoucherSourceContestReward, "x"); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("muốn lỗi rollback có chủ đích, nhận %v", err)
	}
	if n := f.count("user_vouchers", "user_id = ?", user); n != 0 {
		t.Errorf("tx rollback mà còn %d user_vouchers", n)
	}

	err = f.db.Transaction(func(tx *gorm.DB) error {
		_, err := vs.GrantVoucherTx(ctx, tx, user, voucherID, model.UserVoucherSourceContestReward, "Giải")
		return err
	})
	if err != nil {
		t.Fatalf("GrantVoucherTx: %v", err)
	}
	if n := f.count("user_vouchers", "user_id = ? AND voucher_id = ? AND source = ?", user, voucherID, "contest_reward"); n != 1 {
		t.Errorf("muốn 1 user_voucher source=contest_reward, có %d", n)
	}
	if _, err := vs.GrantVoucherTx(ctx, nil, user, voucherID, "contest_reward", ""); err == nil {
		t.Errorf("gọi không có tx phải lỗi")
	}
}

// (e) Voucher tắt / hết hạn / đã xoá / không tồn tại -> ErrVoucherUnavailableForGrant, không ghi gì.
func TestGrantVoucherTx_VoucherKhongDungDuoc(t *testing.T) {
	f := newContestFixture(t)
	vs := newTestVoucherService(f)
	user := f.user("student")
	past := time.Now().Add(-24 * time.Hour)
	deleted := f.voucher(true, nil)
	if err := f.db.Delete(&model.Voucher{}, "id = ?", deleted).Error; err != nil {
		t.Fatalf("xoá mềm voucher: %v", err)
	}
	cases := map[string]uuid.UUID{
		"tắt":            f.voucher(false, nil),
		"hết hạn":        f.voucher(true, &past),
		"đã xoá":         deleted,
		"không tồn tại":  uuid.New(),
	}
	for name, id := range cases {
		err := f.db.Transaction(func(tx *gorm.DB) error {
			_, err := vs.GrantVoucherTx(context.Background(), tx, user, id, "contest_reward", "")
			return err
		})
		if !errors.Is(err, ErrVoucherUnavailableForGrant) {
			t.Errorf("voucher %s: muốn ErrVoucherUnavailableForGrant, nhận %v", name, err)
		}
	}
	if n := f.count("user_vouchers", "user_id = ?", user); n != 0 {
		t.Errorf("voucher không dùng được mà vẫn ghi %d user_vouchers", n)
	}
}

// IssueAwardTx: phát voucher + số chứng nhận đúng khuôn; voucher hỏng -> lỗi để caller rollback cả lần
// chốt, và sau rollback không còn voucher nào đã phát cho người đứng trước trong bảng xếp hạng.
func TestIssueAwardTx_PhatThuong_VoucherHongThiRollbackSach(t *testing.T) {
	f := newContestFixture(t)
	issuer := NewContestRewardService(newTestVoucherService(f), &recordingNotifier{})
	ctx := context.Background()
	first, second := f.user("hang-1"), f.user("hang-2")
	good := f.voucher(true, nil)
	bad := f.voucher(false, nil)
	rank1, rank2 := 1, 2
	contestID := uuid.New()

	var issued *ContestAwardIssued
	err := f.db.Transaction(func(tx *gorm.DB) error {
		var err error
		issued, err = issuer.IssueAwardTx(ctx, tx, ContestAwardGrant{ContestID: contestID, UserID: first, ContestTitle: "Olympic Toán",
			Rank: &rank1, GrantCertificate: true, VoucherID: &good, IssuedAt: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)})
		return err
	})
	if err != nil {
		t.Fatalf("IssueAwardTx: %v", err)
	}
	if issued.CertificateNumber == nil || !regexp.MustCompile(`^CONTEST-20261005-[0-9a-f]{8}$`).MatchString(*issued.CertificateNumber) {
		t.Errorf("số chứng nhận sai khuôn: %v", issued.CertificateNumber)
	}
	var uv model.UserVoucher
	if issued.UserVoucherID == nil || f.db.First(&uv, "id = ?", *issued.UserVoucherID).Error != nil {
		t.Fatalf("không tìm thấy user_voucher đã phát: %v", issued.UserVoucherID)
	}
	if uv.UserID != first || uv.Source != "contest_reward" || uv.Notes != "Giải cuộc thi Olympic Toán - hạng 1" {
		t.Errorf("user_voucher sai: %+v", uv)
	}

	// Lần chốt thứ hai (dữ liệu khác): người hạng 1 nhận voucher tốt, người hạng 2 dính voucher đã tắt.
	third := f.user("hang-1-b")
	err = f.db.Transaction(func(tx *gorm.DB) error {
		if _, err := issuer.IssueAwardTx(ctx, tx, ContestAwardGrant{UserID: third, ContestTitle: "B", Rank: &rank1, VoucherID: &good}); err != nil {
			return err
		}
		_, err := issuer.IssueAwardTx(ctx, tx, ContestAwardGrant{UserID: second, ContestTitle: "B", Rank: &rank2, VoucherID: &bad})
		return err
	})
	if !errors.Is(err, ErrVoucherUnavailableForGrant) {
		t.Fatalf("muốn ErrVoucherUnavailableForGrant, nhận %v", err)
	}
	if n := f.count("user_vouchers", "user_id IN ?", []uuid.UUID{second, third}); n != 0 {
		t.Errorf("lần chốt lỗi mà còn %d user_vouchers phát dở", n)
	}

	certOnly, err := issuer.IssueAwardTx(ctx, f.db, ContestAwardGrant{UserID: second, ContestTitle: "C", GrantCertificate: true})
	if err != nil || certOnly.UserVoucherID != nil || certOnly.CertificateNumber == nil {
		t.Errorf("chứng nhận theo ngưỡng (không voucher): %+v %v", certOnly, err)
	}
	if _, err := issuer.IssueAwardTx(ctx, f.db, ContestAwardGrant{UserID: second}); err == nil {
		t.Errorf("giải không có gì để phát phải lỗi")
	}
}

// recordingNotifier ghi thông báo qua đúng NotificationRepository (như NotificationService, bỏ phần
// đẩy WebSocket), và có thể giả lỗi cho một user.
type recordingNotifier struct {
	repo   *repository.NotificationRepository
	failOn uuid.UUID
}

func (r *recordingNotifier) SendNotification(req dto.CreateNotificationDTO) error {
	if len(req.UserIDs) == 1 && req.UserIDs[0] == r.failOn {
		return errors.New("gửi thất bại giả lập")
	}
	rows := make([]model.Notification, len(req.UserIDs))
	for i, uid := range req.UserIDs {
		rows[i] = model.Notification{UserID: uid, Title: req.Title, Content: req.Content,
			NotificationType: req.NotificationType, ReferenceType: req.ReferenceType, ReferenceID: req.ReferenceID}
	}
	return r.repo.CreateBatch(rows)
}

// Thông báo kết quả: mỗi người một bản, loại achievement, tham chiếu contest; một người gửi lỗi không
// chặn người khác và được phản ánh trong sent + err.
func TestNotifyContestResults_MoiNguoiMotBan_LoiKhongChanNguoiKhac(t *testing.T) {
	f := newContestFixture(t)
	winner, joiner, broken := f.user("winner"), f.user("joiner"), f.user("broken")
	contestID := uuid.New()
	sender := &recordingNotifier{repo: repository.NewNotificationRepository(f.db), failOn: broken}
	issuer := NewContestRewardService(newTestVoucherService(f), sender)
	rank := 1

	sent, err := issuer.NotifyContestResults(context.Background(), []ContestResultNotice{
		{UserID: winner, ContestID: contestID, ContestTitle: "Olympic", Rank: &rank, HasCertificate: true, HasVoucher: true},
		{UserID: broken, ContestID: contestID, ContestTitle: "Olympic"},
		{UserID: joiner, ContestID: contestID, ContestTitle: "Olympic"},
	})
	if sent != 2 || err == nil {
		t.Errorf("muốn sent=2 và có lỗi, nhận sent=%d err=%v", sent, err)
	}
	if n := f.count("notifications", "reference_id = ? AND reference_type = ? AND notification_type = ? AND title = ?",
		contestID, "contest", "achievement", "Kết quả cuộc thi Olympic"); n != 2 {
		t.Errorf("muốn 2 thông báo đúng loại/tham chiếu, có %d", n)
	}
	var n model.Notification
	if err := f.db.First(&n, "user_id = ? AND reference_id = ?", winner, contestID).Error; err != nil {
		t.Fatalf("thông báo của người đạt giải: %v", err)
	}
	if n.Content != "Bạn xếp hạng 1 trong cuộc thi \"Olympic\". Bạn nhận được chứng nhận và voucher, xem voucher trong ví của bạn." {
		t.Errorf("nội dung thông báo: %q", n.Content)
	}
}
