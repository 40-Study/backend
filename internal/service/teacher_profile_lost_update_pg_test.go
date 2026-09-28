package service

// Review PR #73/#30 (plans/reports/review-260928-phase3-pr73-pr30.md, N1 + N3).
//
// N1: PUT /teacher-profiles/:id và PUT bank-info đọc cả dòng rồi GORM Save ghi đè CẢ dòng, không
// khoá. Nếu Resubmit/Approve/Reject (có FOR UPDATE) commit xen giữa lúc đọc và lúc ghi, bản ghi cũ
// ghi đè approval_status, resubmission_count, rejection_reason, reviewed_*.
// Test tái hiện xen kẽ một cách TẤT ĐỊNH: repo bọc chạy thao tác duyệt thật ngay sau lần đọc của
// service, trước lần ghi. Chạy trên Postgres thật trong 1 transaction rồi ROLLBACK (tự skip khi
// không kết nối được) — cùng harness với teacher_profile_resubmit_bypass_pg_test.go.
//
// N3: khôi phục hồ sơ đã xoá mềm thành pending (user đã mất TEACHER) phải xoá reviewed_at/by cũ.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// interleavingProfileRepo chạy afterRead đúng MỘT lần, ngay sau lần đọc đầu tiên của service —
// mô phỏng một transaction duyệt commit trong cửa sổ giữa đọc và ghi.
type interleavingProfileRepo struct {
	*repository.TeacherProfileRepository
	afterRead func()
}

func (r *interleavingProfileRepo) fire() {
	if f := r.afterRead; f != nil {
		r.afterRead = nil
		f()
	}
}

func (r *interleavingProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.TeacherProfile, error) {
	p, err := r.TeacherProfileRepository.GetByID(ctx, id)
	r.fire()
	return p, err
}

func (r *interleavingProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error) {
	p, err := r.TeacherProfileRepository.GetByUserID(ctx, userID)
	r.fire()
	return p, err
}

func reloadProfile(t *testing.T, tx *gorm.DB, id uuid.UUID) model.TeacherProfile {
	t.Helper()
	var p model.TeacherProfile
	if err := tx.Unscoped().Where("id = ?", id).First(&p).Error; err != nil {
		t.Fatal(err)
	}
	return p
}

// Người dùng sửa hồ sơ đúng lúc đang nộp lại: lượt nộp lại (pending, count+1) KHÔNG được bị hoàn tác.
func TestTeacherProfile_PG_UpdateDuringResubmitKeepsResubmission(t *testing.T) {
	tx := bypassPgTx(t)
	ctx := context.Background()
	u := bypassUser(t, tx)
	p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: model.TeacherApprovalRejected,
		ResubmissionCount: 1, RejectionReason: strPtr("Thieu bang cap"), Specialization: strPtr("Toan")}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	appRepo := repository.NewTeacherApplicationRepository(tx)
	repo := &interleavingProfileRepo{TeacherProfileRepository: repository.NewTeacherProfileRepository(tx)}
	repo.afterRead = func() {
		if _, err := appRepo.Resubmit(ctx, u.ID); err != nil {
			t.Fatalf("resubmit xen giua: %v", err)
		}
	}

	resp, err := NewTeacherProfileService(repo).UpdateTeacherProfile(ctx, p.ID, u.ID,
		dto.UpdateTeacherProfileDTO{Specialization: strPtr("Vat ly")})
	if err != nil {
		t.Fatal(err)
	}

	got := reloadProfile(t, tx, p.ID)
	if got.ApprovalStatus != model.TeacherApprovalPending || got.ResubmissionCount != 2 {
		t.Fatalf("LOST UPDATE: status=%s count=%d, muon pending/2", got.ApprovalStatus, got.ResubmissionCount)
	}
	if got.Specialization == nil || *got.Specialization != "Vat ly" {
		t.Fatalf("noi dung sua khong duoc ghi: %v", got.Specialization)
	}
	if resp.ApprovalStatus != model.TeacherApprovalPending || resp.ResubmissionCount != 2 {
		t.Fatalf("response tra trang thai cu: status=%s count=%d", resp.ApprovalStatus, resp.ResubmissionCount)
	}
}

// Admin duyệt đúng lúc người dùng sửa hồ sơ: hồ sơ phải ở approved với reviewed_*; trước bản sửa
// quay về pending trong khi user đã giữ TEACHER.
func TestTeacherProfile_PG_UpdateDuringApproveKeepsApproval(t *testing.T) {
	tx := bypassPgTx(t)
	ctx := context.Background()
	u := bypassUser(t, tx)
	admin := bypassUser(t, tx)
	p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: model.TeacherApprovalPending, Specialization: strPtr("Toan")}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	appRepo := repository.NewTeacherApplicationRepository(tx)
	repo := &interleavingProfileRepo{TeacherProfileRepository: repository.NewTeacherProfileRepository(tx)}
	repo.afterRead = func() {
		if _, err := appRepo.Approve(ctx, u.ID, admin.ID); err != nil {
			t.Fatalf("approve xen giua: %v", err)
		}
	}

	if _, err := NewTeacherProfileService(repo).UpdateTeacherProfile(ctx, p.ID, u.ID,
		dto.UpdateTeacherProfileDTO{Education: strPtr("Thac si")}); err != nil {
		t.Fatal(err)
	}

	got := reloadProfile(t, tx, p.ID)
	if got.ApprovalStatus != model.TeacherApprovalApproved || got.ReviewedBy == nil || *got.ReviewedBy != admin.ID || got.ReviewedAt == nil {
		t.Fatalf("LOST UPDATE: status=%s reviewed_by=%v reviewed_at=%v, muon approved boi admin",
			got.ApprovalStatus, got.ReviewedBy, got.ReviewedAt)
	}
	if got.Education == nil || *got.Education != "Thac si" {
		t.Fatalf("noi dung sua khong duoc ghi: %v", got.Education)
	}
}

// Admin từ chối đúng lúc người dùng sửa hồ sơ: lời từ chối không được bị hoàn tác.
func TestTeacherProfile_PG_UpdateDuringRejectKeepsRejection(t *testing.T) {
	tx := bypassPgTx(t)
	ctx := context.Background()
	u := bypassUser(t, tx)
	admin := bypassUser(t, tx)
	p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: model.TeacherApprovalPending}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	appRepo := repository.NewTeacherApplicationRepository(tx)
	repo := &interleavingProfileRepo{TeacherProfileRepository: repository.NewTeacherProfileRepository(tx)}
	repo.afterRead = func() {
		if _, err := appRepo.Reject(ctx, u.ID, admin.ID, "Chua du kinh nghiem"); err != nil {
			t.Fatalf("reject xen giua: %v", err)
		}
	}

	if _, err := NewTeacherProfileService(repo).UpdateTeacherProfile(ctx, p.ID, u.ID,
		dto.UpdateTeacherProfileDTO{Department: strPtr("Khoa hoc tu nhien")}); err != nil {
		t.Fatal(err)
	}

	got := reloadProfile(t, tx, p.ID)
	if got.ApprovalStatus != model.TeacherApprovalRejected || got.RejectionReason == nil || *got.RejectionReason != "Chua du kinh nghiem" {
		t.Fatalf("LOST UPDATE: status=%s reason=%v, muon rejected kem ly do", got.ApprovalStatus, got.RejectionReason)
	}
}

// Cùng mẫu đọc-rồi-Save ở PUT bank-info (WalletService.UpdateBankInfo).
func TestTeacherProfile_PG_BankInfoUpdateDuringApproveKeepsApproval(t *testing.T) {
	tx := bypassPgTx(t)
	ctx := context.Background()
	u := bypassUser(t, tx)
	admin := bypassUser(t, tx)
	p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: model.TeacherApprovalPending}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	appRepo := repository.NewTeacherApplicationRepository(tx)
	repo := &interleavingProfileRepo{TeacherProfileRepository: repository.NewTeacherProfileRepository(tx)}
	repo.afterRead = func() {
		if _, err := appRepo.Approve(ctx, u.ID, admin.ID); err != nil {
			t.Fatalf("approve xen giua: %v", err)
		}
	}

	err := NewWalletService(nil, repo).UpdateBankInfo(ctx, u.ID, dto.UpdateBankInfoRequest{
		BankName: "VCB", BankAccountNumber: "0123456789", BankAccountName: "NGUYEN VAN A"})
	if err != nil {
		t.Fatal(err)
	}

	got := reloadProfile(t, tx, p.ID)
	if got.ApprovalStatus != model.TeacherApprovalApproved || got.ReviewedBy == nil {
		t.Fatalf("LOST UPDATE: status=%s reviewed_by=%v, muon approved", got.ApprovalStatus, got.ReviewedBy)
	}
	if got.BankAccountNumber == nil || *got.BankAccountNumber != "0123456789" {
		t.Fatalf("bank info khong duoc ghi: %v", got.BankAccountNumber)
	}
}

// N3: hồ sơ đã duyệt, xoá mềm, user không còn TEACHER -> tạo lại thành đơn pending; dấu "đã xét"
// của lần duyệt cũ phải bị xoá để admin không thấy đơn chờ mang reviewed_at/by.
func TestTeacherProfile_PG_RestoreToPendingClearsReviewFields(t *testing.T) {
	tx := bypassPgTx(t)
	ctx := context.Background()
	u := bypassUser(t, tx)
	admin := bypassUser(t, tx)
	reviewedAt := time.Now().Add(-48 * time.Hour)
	p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: model.TeacherApprovalApproved,
		ResubmissionCount: 1, ReviewedBy: &admin.ID, ReviewedAt: &reviewedAt}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Delete(&model.TeacherProfile{}, "id = ?", p.ID).Error; err != nil {
		t.Fatal(err)
	}

	resp, err := NewTeacherProfileService(repository.NewTeacherProfileRepository(tx)).CreateTeacherProfile(ctx,
		dto.CreateTeacherProfileDTO{UserID: u.ID, Specialization: strPtr("Hoa")})
	if err != nil {
		t.Fatal(err)
	}

	got := reloadProfile(t, tx, p.ID)
	if got.DeletedAt.Valid || got.ApprovalStatus != model.TeacherApprovalPending || got.ResubmissionCount != 1 {
		t.Fatalf("khoi phuc sai: deleted=%v status=%s count=%d", got.DeletedAt.Valid, got.ApprovalStatus, got.ResubmissionCount)
	}
	if got.ReviewedAt != nil || got.ReviewedBy != nil {
		t.Fatalf("N3: don pending van mang dau da xet reviewed_at=%v reviewed_by=%v", got.ReviewedAt, got.ReviewedBy)
	}
	if resp.ReviewedAt != nil {
		t.Fatalf("N3: response van tra reviewed_at=%v", *resp.ReviewedAt)
	}
	if got.Specialization == nil || *got.Specialization != "Hoa" {
		t.Fatalf("noi dung moi khong duoc ghi: %v", got.Specialization)
	}
}
