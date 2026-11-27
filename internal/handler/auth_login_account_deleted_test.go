package handler

// Q9-PG-05: nhánh Login của handler cho ErrAccountDeleted trả 401 + code ERR_ACCOUNT_DELETED + thông điệp "đã bị xoá"
// (không phải ACCOUNT_LOCKED "đã bị khoá" của admin) và KHÔNG tính vào bộ đếm lockout (mật khẩu đã đúng).
// Bỏ nhánh ErrAccountDeleted trong AuthHandler.Login -> code về ACCOUNT_LOCKED, test ĐỎ.

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

const testSelfDeletedEmail = "tu-xoa@demo.com"

type fakeAuthServiceSelfDeletedLogin struct {
	service.AuthServiceInterface
}

func (f *fakeAuthServiceSelfDeletedLogin) Login(ctx context.Context, req dto.LoginRequestDto) (*dto.LoginResponseDto, error) {
	if req.Password != testCorrectPassword {
		return nil, errInvalidCredentialTest
	}
	if req.Email == testSelfDeletedEmail {
		return nil, service.ErrAccountDeleted
	}
	return &dto.LoginResponseDto{Completed: true}, nil
}

func TestLogin_TaiKhoanTuXoa_MaRiengVaKhongTangLockout(t *testing.T) {
	app := newRealLoginLockoutTestAppWithService(t, &fakeAuthServiceSelfDeletedLogin{})

	for i := 1; i <= 15; i++ {
		status, body := postLoginWithBody(t, app, validLoginBody(testSelfDeletedEmail, testCorrectPassword))
		if status != fiber.StatusUnauthorized {
			t.Fatalf("lần %d: status=%d, muốn 401 (đúng mật khẩu không được tính vào lockout)", i, status)
		}
		if body["code"] != "ERR_ACCOUNT_DELETED" {
			t.Fatalf("lần %d: code=%v, muốn ERR_ACCOUNT_DELETED", i, body["code"])
		}
		if body["message"] != "Tài khoản đã bị xoá. Liên hệ hỗ trợ nếu muốn khôi phục." {
			t.Fatalf("lần %d: message=%v", i, body["message"])
		}
	}
}
