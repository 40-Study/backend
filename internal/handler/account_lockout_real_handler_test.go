package handler

// Review vòng 4 (sửa lỗ hổng review vòng 3 tự phát hiện, plans/reports/
// claude-260928-0829-review-pr69-account-lockout.md): bản trước của AccountFailureLockout đếm
// MỌI status không phải 2xx là "1 lần thất bại" — kể cả 400 do BodyParser/ValidateStruct (thiếu
// password/device_info/otp trong body). Hậu quả: `{"email":"victim@x.com"}` (thiếu password) lặp
// lại 10 lần là khoá được tài khoản NẠN NHÂN 15 phút mà không cần đoán bất kỳ bí mật nào.
//
// Test này dùng HANDLER THẬT (NewAuthHandler + AuthHandler.Login/Register/ResetPassword thật,
// không mock status code trực tiếp) ghép với AccountFailureLockout thật, để chứng minh: request
// cấu trúc rác/thiếu field không bao giờ chạm tới điểm handler đặt
// middleware.AuthCredentialRejectedLocalsKey (nằm SAU BodyParser+ValidateStruct, ngay tại chỗ
// service từ chối bí mật) nên không bao giờ bị đếm — chỉ 1 lần XÁC THỰC SAI BÍ MẬT THẬT (cấu
// trúc hợp lệ, service so sai) mới bị đếm.

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

// fakeAuthServiceForLockoutTest triển khai đúng 3 method Login/Register/ResetPassword giống
// service thật: kiểm bí mật (mật khẩu/OTP) so với 1 giá trị "đúng" cố định, KHÔNG phân biệt email
// tồn tại hay không (cùng 1 thông báo lỗi chung cho cả 2 trường hợp) — đúng hành vi service thật
// (auth_service.go không tiết lộ "email không tồn tại" khác với "sai mật khẩu").
type fakeAuthServiceForLockoutTest struct {
	service.AuthServiceInterface
}

const (
	testCorrectPassword = "correct-password-123"
	testCorrectOTP      = "123456"
)

var errInvalidCredentialTest = errors.New("invalid email or password")
var errInvalidOTPTest = errors.New("invalid or expired otp")

func (f *fakeAuthServiceForLockoutTest) Login(ctx context.Context, req dto.LoginRequestDto) (*dto.LoginResponseDto, error) {
	if req.Password != testCorrectPassword {
		return nil, errInvalidCredentialTest
	}
	return &dto.LoginResponseDto{Completed: true}, nil
}

func (f *fakeAuthServiceForLockoutTest) Register(ctx context.Context, req dto.VerifyOtpRequestDto) (*dto.RegisterResponseDto, error) {
	if req.OTP != testCorrectOTP {
		return nil, errInvalidOTPTest
	}
	return &dto.RegisterResponseDto{Email: req.Email}, nil
}

func (f *fakeAuthServiceForLockoutTest) ResetPassword(ctx context.Context, req dto.ResetPasswordRequestDto) error {
	if req.Otp != testCorrectOTP {
		return errInvalidOTPTest
	}
	return nil
}

func newRealLoginLockoutTestApp(t *testing.T) *fiber.App {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	h := NewAuthHandler(&fakeAuthServiceForLockoutTest{})
	lockout := middleware.AccountFailureLockout(rdb, middleware.AccountLockoutConfig{KeyPrefix: "t:reallogin", MaxFailures: 10})

	app := fiber.New()
	app.Post("/login", lockout, h.Login)
	return app
}

func postRaw(t *testing.T, app *fiber.App, path, body string) int {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	return resp.StatusCode
}

func validLoginBody(email, password string) string {
	return `{"email":"` + email + `","password":"` + password + `","device_info":{"device_id":"550e8400-e29b-41d4-a716-446655440000","device_name":"test-device","os":"linux"}}`
}

// TestAccountLockout_RealHandler_RequestRacKhongKhoa: kich ban chinh xac coordinator nem ra --
// gui body CHI CO email (thieu password, thieu device_info) 20 lan lien tiep cho CUNG 1 email --
// khong duoc khoa, vi request nay khong bao gio cham toi diem goi service.Login that su (chet o
// ValidateStruct, tra 400 truoc khi toi noi dat AuthCredentialRejectedLocalsKey).
func TestAccountLockout_RealHandler_RequestRacKhongKhoa(t *testing.T) {
	app := newRealLoginLockoutTestApp(t)
	victimEmail := "victim@x.com"
	malformedBody := `{"email":"` + victimEmail + `"}`

	for i := 1; i <= 20; i++ {
		status := postRaw(t, app, "/login", malformedBody)
		if status != fiber.StatusBadRequest {
			t.Fatalf("lan %d: status = %d, muon 400 (Validation failed -- thieu password/device_info)", i, status)
		}
	}

	// Xac nhan tai khoan nan nhan VAN CHUA bi khoa: 1 lan sai mat khau THAT (cau truc hop le)
	// phai tra 401 binh thuong, khong phai 429.
	status := postRaw(t, app, "/login", validLoginBody(victimEmail, "wrong-password"))
	if status != fiber.StatusUnauthorized {
		t.Fatalf("sau 20 request rac: lan dau tien co cau truc hop le (sai mat khau that) status = %d, muon 401 -- tai khoan nan nhan KHONG duoc bi khoa boi 20 request rac truoc do", status)
	}
}

// TestAccountLockout_RealHandler_SaiMatKhauThat_Khoa: 10 lan sai mat khau THAT (cau truc hop le,
// service tu choi that su) -- phai khoa dung sau nguong.
func TestAccountLockout_RealHandler_SaiMatKhauThat_Khoa(t *testing.T) {
	app := newRealLoginLockoutTestApp(t)
	email := "someone@demo.com"

	for i := 1; i <= 10; i++ {
		status := postRaw(t, app, "/login", validLoginBody(email, "wrong-password-"+string(rune('0'+i%10))))
		if status != fiber.StatusUnauthorized {
			t.Fatalf("lan %d: status = %d, muon 401 (chua toi nguong)", i, status)
		}
	}

	status := postRaw(t, app, "/login", validLoginBody(email, "wrong-password-again"))
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("lan 11 (sau 10 lan sai mat khau THAT): status = %d, muon 429", status)
	}
}

// TestAccountLockout_RealHandler_EmailKhongTonTai_DemGiongEmailThat: fake service tra CUNG 1 loi
// cho ca "sai mat khau" lan "email khong ton tai" (dung hanh vi service that) -- xac nhan email
// khong ton tai bi khoa GIONG HET, cung thong bao, khong co nhanh rieng nao lo "email nay khong
// ton tai" (vi du: khong bi khoa nhanh hon/cham hon, khong tra message khac).
func TestAccountLockout_RealHandler_EmailKhongTonTai_DemGiongEmailThat(t *testing.T) {
	app := newRealLoginLockoutTestApp(t)
	nonExistentEmail := "khong-ton-tai-trong-he-thong@demo.com"

	for i := 1; i <= 10; i++ {
		status := postRaw(t, app, "/login", validLoginBody(nonExistentEmail, "bat-ky-mat-khau"))
		if status != fiber.StatusUnauthorized {
			t.Fatalf("lan %d (email khong ton tai): status = %d, muon 401 (giong het email that sai mat khau)", i, status)
		}
	}
	status := postRaw(t, app, "/login", validLoginBody(nonExistentEmail, "bat-ky-mat-khau"))
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("lan 11 (email khong ton tai): status = %d, muon 429 -- phai bi khoa GIONG HET email that, khong duoc co ngoai le tiet lo ton tai", status)
	}
}
