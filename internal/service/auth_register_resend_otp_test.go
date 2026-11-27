package service

// QA4-N07 (contract 8): "Gửi lại mã" ở bước OTP đăng ký phải chạy được chỉ với email (web bỏ mật khẩu khỏi
// sessionStorage). AuthService THẬT + miniredis; chỉ thay bộ gửi mail để bắt mã OTP và repo user giả.
// Mutation đã kiểm: không ghi lại OTPHash mới, hoặc không đặt lại Attempts = 0, trong ResendRegisterOTP làm test ĐỎ.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type resendFakeUserRepo struct {
	repository.UserRepositoryInterface
	created []*model.User
}

func (f *resendFakeUserRepo) FindUserByEmail(context.Context, string) (*model.User, error) {
	return nil, nil
}
func (f *resendFakeUserRepo) UserNameExists(context.Context, string) (bool, error) { return false, nil }
func (f *resendFakeUserRepo) CreateUser(_ context.Context, u *model.User) error {
	f.created = append(f.created, u)
	return nil
}

func TestResendRegisterOTP_EmailOnly_NewCodeWorksOldDoesNot(t *testing.T) {
	mails := make(chan string, 4)
	orig := sendRegisterOTPMail
	sendRegisterOTPMail = func(_ *config.Config, _ string, otp string) error { mails <- otp; return nil }
	t.Cleanup(func() { sendRegisterOTPMail = orig })
	nextOTP := func() string {
		t.Helper()
		select {
		case otp := <-mails:
			return otp
		case <-time.After(2 * time.Second):
			t.Fatal("không có email OTP nào được gửi")
			return ""
		}
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	users := &resendFakeUserRepo{}
	svc := NewAuthService(nil, users, nil, nil, nil, nil, nil, rdb)
	ctx := context.Background()
	email := "resend-otp@40study.test"

	if err := svc.ResendRegisterOTP(ctx, email); !errors.Is(err, ErrRegistrationNotPending) {
		t.Fatalf("chưa đăng ký: err = %v, muốn ErrRegistrationNotPending", err)
	}

	pw := "Demo@12345"
	if err := svc.RequestRegister(ctx, dto.RegisterRequestDto{Email: email, Password: pw, ConfirmPassword: pw, UserName: "resend_otp"}); err != nil {
		t.Fatal(err)
	}
	first := nextOTP()
	// Bốn lần nhập sai trước khi gửi lại (giới hạn là 5): số lần sai phải về 0 sau khi gửi lại.
	wrong := "000000"
	if first == wrong {
		wrong = "111111"
	}
	for i := 0; i < 4; i++ {
		if _, err := svc.Register(ctx, dto.VerifyOtpRequestDto{Email: email, OTP: wrong}); err == nil {
			t.Fatal("OTP sai được chấp nhận")
		}
	}
	if got := pendingAttempts(t, mr, email); got != 4 {
		t.Fatalf("attempts trước khi gửi lại = %d, muốn 4", got)
	}
	mr.FastForward(4 * time.Minute)

	if err := svc.ResendRegisterOTP(ctx, email); err != nil {
		t.Fatalf("gửi lại: %v", err)
	}
	second := nextOTP()
	if got := pendingAttempts(t, mr, email); got != 0 {
		t.Fatalf("attempts sau khi gửi lại = %d, muốn 0", got)
	}
	if ttl := mr.TTL("register:otp:" + email); ttl <= 4*time.Minute {
		t.Fatalf("hạn sau khi gửi lại = %v, muốn tính lại 5 phút", ttl)
	}
	if first != second {
		if _, err := svc.Register(ctx, dto.VerifyOtpRequestDto{Email: email, OTP: first}); err == nil {
			t.Fatal("mã cũ vẫn dùng được sau khi gửi lại")
		}
	}
	resp, err := svc.Register(ctx, dto.VerifyOtpRequestDto{Email: email, OTP: second})
	if err != nil {
		t.Fatalf("mã mới bị từ chối: %v", err)
	}
	if len(users.created) != 1 || resp.Email != email || users.created[0].PasswordHash == "" || users.created[0].PasswordHash == pw {
		t.Fatalf("user tạo ra sai: %+v", users.created)
	}
}

func pendingAttempts(t *testing.T, mr *miniredis.Miniredis, email string) int {
	t.Helper()
	raw, err := mr.Get("register:otp:" + email)
	if err != nil {
		t.Fatalf("đọc bản đăng ký đang chờ: %v", err)
	}
	var p PendingRegistration
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p.Attempts
}

func (f *resendFakeUserRepo) WithUserNameLock(_ context.Context, _ string, fn func(repository.UserRepositoryInterface) error) error {
	return fn(f)
}
