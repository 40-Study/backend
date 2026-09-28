package repository

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// Phase 1 quản lý người dùng (2026-09-28) — bất biến bắt buộc khi khoá tài khoản / gỡ vai trò
// hệ thống. Tách thành hàm THUẦN (không đụng DB) để unit-test được KHÔNG cần Postgres — phần
// khoá dòng (SELECT ... FOR UPDATE, trong transaction) nằm ở user_repository.go /
// user_system_role_repository.go, gọi các hàm này SAU KHI đã lấy dữ liệu đã khoá.
//
// fetchIsActiveByUserIDs (bên dưới) là ngoại lệ DUY NHẤT có đụng DB trong file này — cố ý, để
// CẢ HAI call site (khoá tài khoản, gỡ vai trò hệ thống) đọc `users.is_active` THẬT qua ĐÚNG 1
// hàm, thay vì mỗi nơi tự suy diễn is_active theo cách riêng. Review đối kháng
// (plans/reports/review-260928-users-pr72-pr28.md, finding #1 BLOCKER) phát hiện
// RevokeActiveAssignment từng gán CỨNG is_active=true cho mọi holder SYSTEM_ADMIN — bỏ qua
// tài khoản đã bị khoá trước đó, khiến guard "còn >=1 SYSTEM_ADMIN hoạt động" bị vượt qua bằng
// 1 thao tác gỡ vai trò tuần tự (không cần race).

var (
	// ErrLastSystemAdmin — thao tác sẽ làm hệ thống không còn SYSTEM_ADMIN nào đang hoạt động
	// (khoá tài khoản admin cuối cùng, hoặc gỡ vai trò SYSTEM_ADMIN cuối cùng).
	ErrLastSystemAdmin = errors.New("cannot lock/revoke the last active system admin")
	// ErrLastActiveRoleOfUser — gỡ vai trò này sẽ làm user không còn vai trò hệ thống nào.
	ErrLastActiveRoleOfUser = errors.New("cannot revoke the last active system role of this user")
)

// evaluateLastSystemAdminGuard nhận danh sách userID đang giữ vai trò SYSTEM_ADMIN active
// (đã SELECT ... FOR UPDATE để tuần tự hoá 2 thao tác đồng thời) và map is_active hiện tại của
// từng user đó (is_active=true nghĩa là admin còn ĐĂNG NHẬP ĐƯỢC — "đang hoạt động thật").
// targetUserID là user SẮP bị khoá hoặc SẮP bị gỡ vai trò SYSTEM_ADMIN — hàm coi targetUserID
// như đã "mất tư cách hoạt động" và đếm phần còn lại.
//
// Trả ErrLastSystemAdmin nếu sau thao tác, không còn ai khác đang hoạt động.
func evaluateLastSystemAdminGuard(
	activeAdminUserIDs []uuid.UUID,
	activeStatusByUserID map[uuid.UUID]bool,
	targetUserID uuid.UUID,
) error {
	remaining := 0
	for _, id := range activeAdminUserIDs {
		if id == targetUserID {
			continue
		}
		if activeStatusByUserID[id] {
			remaining++
		}
	}
	if remaining == 0 {
		return ErrLastSystemAdmin
	}
	return nil
}

// evaluateLastActiveRoleGuard chặn gỡ vai trò cuối cùng của 1 user — activeRoleCount là số
// vai trò hệ thống ĐANG active của user đó TRƯỚC KHI gỡ (bao gồm cả vai trò sắp gỡ).
func evaluateLastActiveRoleGuard(activeRoleCount int) error {
	if activeRoleCount <= 1 {
		return ErrLastActiveRoleOfUser
	}
	return nil
}

// fetchIsActiveByUserIDs trả về map is_active THẬT từ bảng `users` cho đúng tập userID truyền
// vào, đọc qua CHÍNH transaction `tx` (thấy được dữ liệu đã SELECT ... FOR UPDATE ở lượt khoá
// dòng trước đó trong cùng transaction, nếu có). Dùng cho CẢ HAI đường "khoá tài khoản"
// (user_repository.go) và "gỡ vai trò hệ thống" (user_system_role_repository.go) — một user còn
// giữ vai trò SYSTEM_ADMIN active không đồng nghĩa tài khoản của họ đang is_active=true (đã bị
// khoá trước đó bởi một thao tác khác); phải hỏi DB, không được suy diễn/gán cứng.
func fetchIsActiveByUserIDs(tx *gorm.DB, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	activeStatus := make(map[uuid.UUID]bool, len(userIDs))
	if len(userIDs) == 0 {
		return activeStatus, nil
	}
	var rows []model.User
	if err := tx.Select("id", "is_active").Where("id IN ?", userIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		activeStatus[row.ID] = row.IsActive
	}
	return activeStatus, nil
}
