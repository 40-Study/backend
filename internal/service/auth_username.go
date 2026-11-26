package service

// auth_username.go — QA8 PG-02 (C19): tên đăng nhập là thông tin công khai (bảng xếp hạng, hồ sơ) nên không được có hai
// tài khoản cùng tên (so không phân biệt hoa thường) và không ai được đăng ký/đổi sang tên dành riêng như "admin".
// KHÔNG có unique index trên lower(user_name) ở vòng này: seed hiện có 8 cặp tên chỉ khác miền email (backlog dọn seed trước).

import (
	"context"
	"fmt"
	"strings"

	"study.com/v1/internal/apperr"
)

// ErrUserNameCode: mã lỗi ổn định cho tên đăng nhập đã dùng / dành riêng; web hiện dưới ô tên đăng nhập.
const ErrUserNameCode = "ERR_USERNAME_EXISTS"

var (
	// ErrUserNameTaken: đã có tài khoản khác dùng tên này.
	ErrUserNameTaken = apperr.ConflictCode(ErrUserNameCode, "Tên đăng nhập đã được dùng. Vui lòng chọn tên khác.")
	// ErrUserNameReserved: tên trùng một tên dành riêng của hệ thống.
	ErrUserNameReserved = apperr.ConflictCode(ErrUserNameCode, "Tên đăng nhập này được dành riêng. Vui lòng chọn tên khác.")
)

// reservedUserNames: tên người dùng thường không được lấy (so sau khi viết thường). Tài khoản ĐANG mang tên này (seed)
// vẫn giữ được: xem ensureUserNameAvailable.
var reservedUserNames = map[string]struct{}{
	"admin": {}, "root": {}, "support": {}, "fortex": {}, "40study": {},
}

// ensureUserNameAvailable: nil khi `name` dùng được. `ownName` là tên hiện tại của CHÍNH tài khoản đang thao tác
// ("" khi tạo mới): đổi hoa/thường tên của mình hoặc giữ nguyên tên (kể cả tên dành riêng của seed) không bị chặn.
func (s *AuthService) ensureUserNameAvailable(ctx context.Context, name, ownName string) error {
	name = strings.TrimSpace(name)
	if ownName != "" && strings.EqualFold(name, strings.TrimSpace(ownName)) {
		return nil
	}
	if _, reserved := reservedUserNames[strings.ToLower(name)]; reserved {
		return ErrUserNameReserved
	}
	taken, err := s.userRepo.UserNameExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check user name: %w", err)
	}
	if taken {
		return ErrUserNameTaken
	}
	return nil
}
