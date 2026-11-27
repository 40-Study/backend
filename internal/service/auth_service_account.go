package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

// ErrInvalidDeletePassword (#142 M1): DELETE /auth/me sai mật khẩu -> 400 ERR_INVALID_PASSWORD. Giữ message "invalid password"
// vì web cũ nhận ra mật khẩu sai qua câu này.
var ErrInvalidDeletePassword = apperr.BadRequestCode("ERR_INVALID_PASSWORD", "invalid password")

// ErrAccountHasBalance (QA8 T10, C22): giảng viên còn số dư ví hoặc yêu cầu rút tiền đang xử lý thì chưa xoá được tài khoản
// (xoá = khoá đăng nhập, tiền còn lại không rút được nữa). Mã ổn định để web hiện đúng hướng dẫn.
var ErrAccountHasBalance = apperr.ConflictCode("ERR_ACCOUNT_HAS_BALANCE",
	"Ví của bạn còn số dư hoặc còn yêu cầu rút tiền đang xử lý. Hãy rút hết và chờ yêu cầu hoàn tất trước khi xoá tài khoản.")

// TeacherMoneyReader: phần đọc ví giảng viên mà DeleteAccount cần (repository.WalletRepository thoả mãn).
type TeacherMoneyReader interface {
	GetTeacherEarnings(teacherID uuid.UUID) (*repository.TeacherEarningsRow, error)
	GetTeacherPayoutSums(teacherID uuid.UUID) (repository.TeacherPayoutSums, error)
}

// ensureNoTeacherMoney trả ErrAccountHasBalance khi số dư khả dụng > 0 (cùng công thức availableBalance của ví) hoặc còn yêu
// cầu rút pending/approved. Người dùng không có khoá học bán được có thu nhập 0 nên không bị chặn.
func (s *AuthService) ensureNoTeacherMoney(userID uuid.UUID) error {
	if s.teacherMoney == nil {
		return errors.New("teacher money reader is not configured")
	}
	earnings, err := s.teacherMoney.GetTeacherEarnings(userID)
	if err != nil {
		return fmt.Errorf("failed to read teacher earnings: %w", err)
	}
	sums, err := s.teacherMoney.GetTeacherPayoutSums(userID)
	if err != nil {
		return fmt.Errorf("failed to read teacher payouts: %w", err)
	}
	if sums.Open.IsPositive() || availableBalance(earnings.TotalEarnings, sums).IsPositive() {
		return ErrAccountHasBalance
	}
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
		return ErrInvalidDeletePassword
	}

	// 3. Còn tiền trong ví giảng viên thì chưa cho xoá (sau bước mật khẩu để không lộ tình trạng ví cho người lạ)
	if err := s.ensureNoTeacherMoney(userID); err != nil {
		return err
	}

	// 4. Soft-delete: mark inactive
	now := time.Now()
	user.IsActive = false
	user.SelfDeletedAt = &now // Q9-PG-05: Login dựa vào dấu này để báo "đã bị xoá" thay vì "đã bị khoá"
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to deactivate account: %w", err)
	}

	// 5. Revoke all sessions
	if err := s.LogoutAllDevice(ctx, userID); err != nil {
		log.Printf("[WARN] Failed to logout all devices for user %s: %v", userID, err)
	}

	return nil
}
