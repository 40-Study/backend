package service

// Review đối kháng PR #73 (MAJOR #2): chủ hồ sơ lách giới hạn 3 lần nộp lại (quyết định #5) bằng
// DELETE /teacher-profiles/:id?hard_delete=true rồi POST /teacher-profiles — hồ sơ mới pending với
// resubmission_count=0. Test chạy trên Postgres THẬT trong 1 transaction rồi ROLLBACK (tự skip khi
// không kết nối được), đi đúng đường service + repository thật.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func bypassPgTx(t *testing.T) *gorm.DB {
	t.Helper()
	db := openTestPostgresForPaymentTest(t)
	if !db.Migrator().HasColumn(&model.TeacherProfile{}, "resubmission_count") {
		t.Skip("DB chua co cot phase 3")
	}
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })
	return tx
}

func bypassUser(t *testing.T, tx *gorm.DB) model.User {
	t.Helper()
	s := uuid.NewString()
	u := model.User{Email: "bypass-" + s + "@40study.test", PasswordHash: "x", UserName: "bypass-" + s}
	if err := tx.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

// Đúng đường lách của review: hồ sơ đã dùng hết 3 lần nộp lại và bị từ chối lần 4.
func TestTeacherProfile_PG_CannotBypassResubmitLimitByDeleteAndRecreate(t *testing.T) {
	tx := bypassPgTx(t)
	ctx := context.Background()
	u := bypassUser(t, tx)
	reason := "Van thieu minh chung"
	p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: model.TeacherApprovalRejected,
		ResubmissionCount: model.MaxTeacherResubmissions, RejectionReason: &reason}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewTeacherProfileService(repository.NewTeacherProfileRepository(tx))
	appRepo := repository.NewTeacherApplicationRepository(tx)

	for _, hard := range []bool{true, false} {
		if err := svc.DeleteTeacherProfile(ctx, p.ID, u.ID, hard); err == nil {
			t.Errorf("chu ho so xoa duoc ho so dang bi tu choi (hard=%v) — mo duong lach gioi han", hard)
		}
	}
	spec := "Toan"
	if created, err := svc.CreateTeacherProfile(ctx, dto.CreateTeacherProfileDTO{UserID: u.ID, Specialization: &spec}); err == nil &&
		created.ResubmissionCount == 0 && created.ApprovalStatus == model.TeacherApprovalPending {
		t.Fatalf("LACH GIOI HAN: ho so moi status=%s resubmission_count=%d", created.ApprovalStatus, created.ResubmissionCount)
	}
	if _, err := appRepo.Resubmit(ctx, u.ID); !errors.Is(err, repository.ErrTeacherResubmissionLimit) {
		t.Fatalf("sau cac lan thu lach, nop lai van phai bi chan boi gioi han, err=%v", err)
	}
}

// Hồ sơ ĐÃ duyệt được phép xoá mềm; tạo lại phải KHÔI PHỤC đúng dòng cũ (giữ resubmission_count),
// không tạo dòng mới với bộ đếm 0 (vd user sau đó bị gỡ TEACHER rồi tạo lại để nộp đơn mới).
func TestTeacherProfile_PG_RecreateAfterSoftDeleteKeepsCounter(t *testing.T) {
	tx := bypassPgTx(t)
	ctx := context.Background()
	u := bypassUser(t, tx)
	p := model.TeacherProfile{UserID: u.ID, ApprovalStatus: model.TeacherApprovalApproved, ResubmissionCount: 2}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewTeacherProfileService(repository.NewTeacherProfileRepository(tx))
	// Xoá VĨNH VIỄN bị cấm kể cả khi hồ sơ đã duyệt: mất dòng = mất bộ đếm nộp lại.
	if err := svc.DeleteTeacherProfile(ctx, p.ID, u.ID, true); !errors.Is(err, ErrTeacherProfileHardDeleteForbidden) {
		t.Fatalf("hard_delete cua chu ho so phai bi cam, err=%v", err)
	}
	var still int64
	tx.Model(&model.TeacherProfile{}).Where("id = ?", p.ID).Count(&still)
	if still != 1 {
		t.Fatal("ho so bi xoa du hard_delete bi cam")
	}
	if err := svc.DeleteTeacherProfile(ctx, p.ID, u.ID, false); err != nil {
		t.Fatalf("xoa mem ho so da duyet phai duoc: %v", err)
	}
	spec := "Ly"
	created, err := svc.CreateTeacherProfile(ctx, dto.CreateTeacherProfileDTO{UserID: u.ID, Specialization: &spec})
	if err != nil {
		t.Fatalf("tao lai sau xoa mem: %v", err)
	}
	if created.ID != p.ID || created.ResubmissionCount != 2 {
		t.Fatalf("tao lai phai khoi phuc dong cu va giu bo dem: id=%s (cu %s) count=%d", created.ID, p.ID, created.ResubmissionCount)
	}
	if created.Specialization == nil || *created.Specialization != "Ly" {
		t.Fatalf("tao lai phai cap nhat noi dung moi: %+v", created.Specialization)
	}
}
