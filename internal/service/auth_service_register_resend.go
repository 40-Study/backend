package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/utils"
)

// ErrRegistrationNotPending (QA4-N07): không còn đăng ký đang chờ OTP cho email này (hết 5 phút, đã xác thực
// xong, hoặc chưa từng gửi). Handler trả 410 ERR_REGISTRATION_EXPIRED.
var ErrRegistrationNotPending = errors.New("Phiên đăng ký đã hết hạn. Vui lòng đăng ký lại.")

// sendRegisterOTPMail: điểm thay được trong test (mã OTP chỉ tồn tại trong email gửi đi).
var sendRegisterOTPMail = utils.SendRegisterOTP

// ResendRegisterOTP (QA4-N07, contract 8) gửi lại mã OTP đăng ký từ bản đăng ký đang chờ mà SERVER giữ
// (register:otp:{email}), nên web không còn phải giữ mật khẩu trong sessionStorage để gọi lại
// /register/request. Mã cũ hết hiệu lực, số lần nhập sai về 0, hạn 5 phút tính lại từ bây giờ. Số lần gửi bị
// OTPRateLimiter giới hạn theo email ở router (dùng chung bộ đếm với /register/request).
func (s *AuthService) ResendRegisterOTP(ctx context.Context, email string) error {
	pendingKey := constants.KeyRegisterOTP(email)
	raw, err := s.redisClient.Get(ctx, pendingKey).Result()
	if err == redis.Nil {
		return ErrRegistrationNotPending
	}
	if err != nil {
		return fmt.Errorf("failed to get pending registration: %w", err)
	}
	var pending PendingRegistration
	if err := json.Unmarshal([]byte(raw), &pending); err != nil {
		return fmt.Errorf("failed to parse pending registration: %w", err)
	}

	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}
	pending.OTPHash = utils.HashOTP(otp, pending.Email)
	pending.Attempts = 0
	updated, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("failed to marshal pending data: %w", err)
	}
	if err := s.redisClient.Set(ctx, pendingKey, updated, registerOTPTTL).Err(); err != nil {
		return fmt.Errorf("failed to save pending registration: %w", err)
	}

	to := pending.Email
	utils.SafeGo(func() {
		if err := sendRegisterOTPMail(s.cfg, to, otp); err != nil {
			log.Printf("[WARN] Failed to resend register OTP email to %s: %v", to, err)
		}
	})
	return nil
}
