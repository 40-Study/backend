package handler

// Merge main vào PR #72 (review vòng 2, plans/reports/review-260928-users-round2-pr72-pr28.md mục 3):
// Login có 2 nhánh lỗi — ErrUserInactive (tài khoản bị khoá, mật khẩu ĐÃ ĐÚNG) và lỗi generic
// (sai email/mật khẩu). Chỉ nhánh generic được đặt middleware.AuthCredentialRejectedLocalsKey.
// Nếu đảo thứ tự (đặt cờ trước khi kiểm ErrUserInactive), mỗi lần gõ ĐÚNG mật khẩu vào tài khoản
// bị khoá sẽ bị AccountFailureLockout đếm như một lần đoán sai, và sau MaxFailures lần sẽ trả 429.

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

const testLockedAccountEmail = "bi-khoa@demo.com"

// fakeAuthServiceInactiveLogin giống service thật: kiểm mật khẩu TRƯỚC, kiểm IsActive SAU
// (auth_service.go). Tài khoản testLockedAccountEmail có mật khẩu đúng nhưng đã bị khoá.
type fakeAuthServiceInactiveLogin struct {
	service.AuthServiceInterface
}

func (f *fakeAuthServiceInactiveLogin) Login(ctx context.Context, req dto.LoginRequestDto) (*dto.LoginResponseDto, error) {
	if req.Password != testCorrectPassword {
		return nil, errInvalidCredentialTest
	}
	if req.Email == testLockedAccountEmail {
		return nil, service.ErrUserInactive
	}
	return &dto.LoginResponseDto{Completed: true}, nil
}

func postLoginWithBody(t *testing.T, app *fiber.App, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	parsed := map[string]any{}
	_ = json.Unmarshal(raw, &parsed)
	return resp.StatusCode, parsed
}

// TestLogin_TaiKhoanBiKhoaDungMatKhau_KhongTangBoDemLockout: gõ ĐÚNG mật khẩu vào tài khoản bị
// khoá nhiều hơn MaxFailures (10) lần — mọi lần phải trả 401 ACCOUNT_LOCKED, không lần nào 429.
// Sau đó, 10 lần sai mật khẩu thật vẫn còn đủ "hạn mức" (401), lần 11 mới 429 — chứng minh
// bộ đếm không hề bị tăng bởi các lần đăng nhập đúng mật khẩu trước đó.
func TestLogin_TaiKhoanBiKhoaDungMatKhau_KhongTangBoDemLockout(t *testing.T) {
	app := newRealLoginLockoutTestAppWithService(t, &fakeAuthServiceInactiveLogin{})

	for i := 1; i <= 15; i++ {
		status, body := postLoginWithBody(t, app, validLoginBody(testLockedAccountEmail, testCorrectPassword))
		if status != fiber.StatusUnauthorized {
			t.Fatalf("lan %d (dung mat khau, tai khoan bi khoa): status = %d, muon 401 -- lan dang nhap dung mat khau KHONG duoc dem vao bo dem lockout", i, status)
		}
		if body["code"] != "ACCOUNT_LOCKED" {
			t.Fatalf("lan %d: code = %v, muon ACCOUNT_LOCKED", i, body["code"])
		}
	}

	for i := 1; i <= 10; i++ {
		status, _ := postLoginWithBody(t, app, validLoginBody(testLockedAccountEmail, "wrong-password"))
		if status != fiber.StatusUnauthorized {
			t.Fatalf("sai mat khau lan %d sau 15 lan dung mat khau: status = %d, muon 401 -- bo dem da bi tang boi cac lan dung mat khau", i, status)
		}
	}
	status, _ := postLoginWithBody(t, app, validLoginBody(testLockedAccountEmail, "wrong-password"))
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("sai mat khau lan 11: status = %d, muon 429 (nguong van hoat dong cho sai mat khau that)", status)
	}
}
