package service

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"study.com/v1/internal/dto"
)

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
