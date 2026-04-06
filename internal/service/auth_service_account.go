package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

const changeEmailOTPTTL = 10 * time.Minute

// RequestChangeEmail verifies the user's password, checks new email availability,
// generates OTP, stores it in Redis, and sends it via email.
func (s *AuthService) RequestChangeEmail(ctx context.Context, userID uuid.UUID, req dto.RequestChangeEmailDto) error {
	// 1. Get user
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}
	if user == nil {
		return errors.New("user not found")
	}

	// 2. Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return errors.New("invalid password")
	}

	// 3. Check new email is not already taken
	existing, err := s.userRepo.FindUserByEmail(ctx, req.NewEmail)
	if err != nil {
		return fmt.Errorf("failed to check email availability: %w", err)
	}
	if existing != nil {
		return errors.New("email already in use")
	}

	// 4. Generate OTP
	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	// 5. Store OTP + new email in Redis: value = "{otp}:{new_email}"
	otpKey := constants.KeyChangeEmailOTP(userID.String())
	value := fmt.Sprintf("%s:%s", otp, req.NewEmail)
	if err := s.redisClient.Set(ctx, otpKey, value, changeEmailOTPTTL).Err(); err != nil {
		return fmt.Errorf("failed to store OTP: %w", err)
	}

	// 6. Send OTP via email (best-effort, non-blocking)
	go func() {
		if err := utils.SendResetPasswordOTP(s.cfg, req.NewEmail, otp); err != nil {
			log.Printf("[WARN] Failed to send change-email OTP to %s: %v", req.NewEmail, err)
		}
	}()

	return nil
}

// ChangeEmail validates the OTP and updates the user's email address.
func (s *AuthService) ChangeEmail(ctx context.Context, userID uuid.UUID, req dto.ChangeEmailDto) error {
	// 1. Retrieve stored OTP value from Redis
	otpKey := constants.KeyChangeEmailOTP(userID.String())
	stored, err := s.redisClient.Get(ctx, otpKey).Result()
	if err == redis.Nil {
		return errors.New("OTP expired or not found, please request again")
	}
	if err != nil {
		return fmt.Errorf("failed to retrieve OTP: %w", err)
	}

	// 2. Parse stored value: "{otp}:{new_email}"
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return errors.New("invalid OTP data, please request again")
	}
	storedOTP, storedEmail := parts[0], parts[1]

	// 3. Verify OTP and email match
	if storedOTP != req.OTP {
		return errors.New("invalid OTP")
	}
	if storedEmail != req.NewEmail {
		return errors.New("email does not match the requested change")
	}

	// 4. Update user email
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}
	if user == nil {
		return errors.New("user not found")
	}
	user.Email = req.NewEmail
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update email: %w", err)
	}

	// 5. Invalidate OTP
	_ = s.redisClient.Del(ctx, otpKey).Err()

	return nil
}

// DeleteAccount verifies the user's password then soft-deletes the account
// by setting IsActive = false and revoking all sessions.
func (s *AuthService) DeleteAccount(ctx context.Context, userID uuid.UUID, req dto.DeleteAccountDto) error {
	// 1. Get user
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}
	if user == nil {
		return errors.New("user not found")
	}

	// 2. Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return errors.New("invalid password")
	}

	// 3. Soft-delete: mark inactive
	user.IsActive = false
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to deactivate account: %w", err)
	}

	// 4. Revoke all sessions
	if err := s.LogoutAllDevice(ctx, userID); err != nil {
		log.Printf("[WARN] Failed to logout all devices for user %s: %v", userID, err)
	}

	return nil
}
