package service

// Q9-PG-05: đăng nhập vào tài khoản TỰ xoá (DeleteAccount ghi users.self_deleted_at) phải khác mọi tài khoản không hoạt động
// khác: admin khoá (locked_by/locked_at) và admin xoá mềm giảng viên (chỉ is_active=false, không dấu nào) đều vẫn là "bị khoá".
// Bỏ nhánh isSelfDeactivated khỏi Login, hoặc quay lại suy luận "không có locked_*" -> test ĐỎ (ca giảng viên bị admin xoá mềm).

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLogin_TaiKhoanTuXoa_BaoDaBiXoa_KhacAdminKhoa(t *testing.T) {
	const password = "Demo@123"
	svc, userRepo := newLastLoginTestService(t, password)
	req := lastLoginRequest(password)

	// Tự xoá: is_active=false + self_deleted_at do DeleteAccount ghi.
	selfDeletedAt := time.Now()
	userRepo.user.IsActive = false
	userRepo.user.SelfDeletedAt = &selfDeletedAt
	_, err := svc.Login(context.Background(), req)
	if !errors.Is(err, ErrAccountDeleted) {
		t.Fatalf("tự xoá: err=%v, muốn ErrAccountDeleted", err)
	}
	if !errors.Is(err, ErrUserInactive) {
		t.Fatalf("ErrAccountDeleted phải vẫn là ErrUserInactive (SwitchOrg/lockout dựa vào), err=%v", err)
	}

	// Admin khoá: locked_by + locked_at được ghi, không có self_deleted_at -> vẫn là "bị khoá".
	adminID, now := uuid.New(), time.Now()
	userRepo.user.SelfDeletedAt = nil
	userRepo.user.LockedBy, userRepo.user.LockedAt = &adminID, &now
	_, err = svc.Login(context.Background(), req)
	if !errors.Is(err, ErrUserInactive) || errors.Is(err, ErrAccountDeleted) {
		t.Fatalf("admin khoá: err=%v, muốn ErrUserInactive và KHÔNG phải ErrAccountDeleted", err)
	}

	// Admin xoá mềm giảng viên (DELETE /api/teachers/:id): chỉ is_active=false, không locked_*, không self_deleted_at
	// -> KHÔNG được báo "tự xoá" (#158 M1). Cùng ca với dòng cũ trước khi có locked_* (#158 L1).
	userRepo.user.LockedBy, userRepo.user.LockedAt = nil, nil
	_, err = svc.Login(context.Background(), req)
	if !errors.Is(err, ErrUserInactive) || errors.Is(err, ErrAccountDeleted) {
		t.Fatalf("admin xoá mềm giảng viên: err=%v, muốn ErrUserInactive và KHÔNG phải ErrAccountDeleted", err)
	}

	// Sai mật khẩu vào tài khoản tự xoá: không lộ trạng thái xoá, vẫn là lỗi thông tin đăng nhập chung.
	userRepo.user.SelfDeletedAt = &selfDeletedAt
	_, err = svc.Login(context.Background(), lastLoginRequest("sai-mat-khau"))
	if err == nil || errors.Is(err, ErrUserInactive) {
		t.Fatalf("sai mật khẩu: err=%v, không được lộ trạng thái tài khoản", err)
	}
}
