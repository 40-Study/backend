package service

// Q9-PG-05: đăng nhập vào tài khoản TỰ xoá (DELETE /auth/me chỉ đặt is_active=false) phải khác với tài khoản bị admin khoá
// (locked_by/locked_at do LockOrUnlockUser ghi). Bỏ nhánh isSelfDeactivated khỏi Login -> test ĐỎ.

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

	// Tự xoá: is_active=false, không có dấu vết admin.
	userRepo.user.IsActive = false
	_, err := svc.Login(context.Background(), req)
	if !errors.Is(err, ErrAccountDeleted) {
		t.Fatalf("tự xoá: err=%v, muốn ErrAccountDeleted", err)
	}
	if !errors.Is(err, ErrUserInactive) {
		t.Fatalf("ErrAccountDeleted phải vẫn là ErrUserInactive (SwitchOrg/lockout dựa vào), err=%v", err)
	}

	// Admin khoá: locked_by + locked_at được ghi -> vẫn là "bị khoá".
	adminID, now := uuid.New(), time.Now()
	userRepo.user.LockedBy, userRepo.user.LockedAt = &adminID, &now
	_, err = svc.Login(context.Background(), req)
	if !errors.Is(err, ErrUserInactive) || errors.Is(err, ErrAccountDeleted) {
		t.Fatalf("admin khoá: err=%v, muốn ErrUserInactive và KHÔNG phải ErrAccountDeleted", err)
	}

	// Sai mật khẩu vào tài khoản tự xoá: không lộ trạng thái xoá, vẫn là lỗi thông tin đăng nhập chung.
	userRepo.user.LockedBy, userRepo.user.LockedAt = nil, nil
	_, err = svc.Login(context.Background(), lastLoginRequest("sai-mat-khau"))
	if err == nil || errors.Is(err, ErrUserInactive) {
		t.Fatalf("sai mật khẩu: err=%v, không được lộ trạng thái tài khoản", err)
	}
}
