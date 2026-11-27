package service

// Q9-PG-05 / #158 M1 (Postgres THẬT, schema tạm): dấu users.self_deleted_at.
//   - DeleteAccount ghi dấu và Login báo ErrAccountDeleted; bỏ dòng ghi dấu trong DeleteAccount -> ĐỎ.
//   - Admin xoá mềm giảng viên (TeacherRepository.DeleteTeacher) KHÔNG ghi dấu -> Login vẫn ErrUserInactive "bị khoá".
//   - Admin mở khoá (LockOrUnlockUser isActive=true) xoá dấu; bỏ dòng xoá dấu trong LockOrUnlockUser -> ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

func TestQ9_PG_SelfDeletedMarker_GhiKhiTuXoa_KhongGhiKhiAdminXoaMem_XoaKhiMoKhoa(t *testing.T) {
	f := newWithdrawalFixture(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(f.db)
	svc := NewAuthService(nil, userRepo, nil, nil, nil, nil, nil, nil)
	svc.SetTeacherMoneyReader(repository.NewWalletRepository(f.db))

	hash, err := utils.HashPassword(qa8Password)
	if err != nil {
		t.Fatal(err)
	}
	load := func(id uuid.UUID) model.User {
		var u model.User
		if err := f.db.First(&u, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		return u
	}

	// 1. Tự xoá -> is_active=false + self_deleted_at.
	self := f.user("self-delete")
	if err := f.db.Model(&model.User{}).Where("id = ?", self).Update("password_hash", hash).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAccount(ctx, self, dto.DeleteAccountDto{Password: qa8Password}); err != nil {
		t.Fatalf("tự xoá: %v", err)
	}
	u := load(self)
	if u.IsActive || u.SelfDeletedAt == nil {
		t.Fatalf("sau DeleteAccount: is_active=%v self_deleted_at=%v, muốn false + có giá trị", u.IsActive, u.SelfDeletedAt)
	}
	if !isSelfDeactivated(&u) {
		t.Fatal("isSelfDeactivated phải đúng với tài khoản vừa tự xoá")
	}

	// 2. Admin xoá mềm giảng viên: không dấu nào -> không được coi là tự xoá.
	teacher := f.teacher(false)
	if err := repository.NewTeacherRepository(f.db).DeleteTeacher(ctx, teacher, false); err != nil {
		t.Fatalf("xoá mềm giảng viên: %v", err)
	}
	tu := load(teacher)
	if tu.IsActive || tu.SelfDeletedAt != nil || isSelfDeactivated(&tu) {
		t.Fatalf("admin xoá mềm giảng viên: is_active=%v self_deleted_at=%v selfDeactivated=%v, muốn false/nil/false",
			tu.IsActive, tu.SelfDeletedAt, isSelfDeactivated(&tu))
	}

	// 3. Mở khoá/kích hoạt lại xoá dấu tự xoá.
	if _, err := userRepo.LockOrUnlockUser(ctx, self, true, nil, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("mở khoá: %v", err)
	}
	u = load(self)
	if !u.IsActive || u.SelfDeletedAt != nil {
		t.Fatalf("sau mở khoá: is_active=%v self_deleted_at=%v, muốn true + nil", u.IsActive, u.SelfDeletedAt)
	}
}
