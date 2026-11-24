package service

// QA6-PG-02 (C7): xác thực OTP đăng ký khi phiên chờ đã hết hạn/không còn phải trả ErrRegistrationNotPending
// (handler map sang 410 ERR_REGISTRATION_EXPIRED). Trước đây là errors.New riêng nên handler trả 400 và web không dọn phiên.
// Đổi lại thành errors.New(...) trong AuthService.Register thì test ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/dto"
)

func TestQA6_RegisterVerify_PhienDaHetHan_TraErrRegistrationNotPending(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	svc := NewAuthService(nil, nil, nil, nil, nil, nil, nil, rdb)

	_, err := svc.Register(context.Background(), dto.VerifyOtpRequestDto{Email: "expired-reg@40study.test", OTP: "123456"})
	if !errors.Is(err, ErrRegistrationNotPending) {
		t.Fatalf("err = %v, muốn ErrRegistrationNotPending", err)
	}
}
