package database

// r2_social_privacy_cleanup.go — ba bước SỬA MỘT LẦN dữ liệu cũ của đợt sửa lỗi sau kiểm thử hồi quy 03/10/2026
// (lane R2). Mỗi bước có bản ghi đánh dấu trong data_migrations nên chỉ chạy đúng một lần trên mỗi DB:
//
//   - A-03: hội thoại nhóm mồ côi. Trước bản sửa DeleteGroup chỉ xoá nhóm, hội thoại GROUP của nó vẫn hiện trong
//     danh sách và gửi tin được. Đóng các hội thoại GROUP mà nhóm đã xoá mềm hoặc không còn.
//   - B-03: user_name chứa '@' (người dùng gõ cả email lúc đăng ký) lộ email ở bảng xếp hạng công khai và hồ sơ
//     công khai. Đổi sang tên sinh từ họ tên không dấu + hậu tố ngẫu nhiên, KHÔNG lấy từ email, giữ duy nhất.
//   - A-14: thông báo "đã gửi lời mời kết bạn" của lời mời không còn hiệu lực (đã huỷ/từ chối/chấp nhận/chặn).

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

const (
	r2OrphanGroupConversationsName = "r2_close_orphan_group_conversations"
	r2UserNameWithAtRenameName     = "r2_rename_user_name_containing_at"
	r2ResolvedFriendNoticesName    = "r2_delete_resolved_friend_request_notices"

	// r2CleanupLockKey: khoá advisory (toàn DB) cho các bước sửa một lần của lane R2; khác mọi khoá khác trong gói.
	r2CleanupLockKey int64 = 4020261004

	r2RenameBatch = 500
)

// runR2SocialPrivacyCleanup gọi từ RunPostMigrations sau AutoMigrate (bảng conversations/users đã có).
func runR2SocialPrivacyCleanup(db *gorm.DB) error {
	if err := runDataMigrationOnce(db, r2OrphanGroupConversationsName, closeOrphanGroupConversations); err != nil {
		return err
	}
	if err := runDataMigrationOnce(db, r2UserNameWithAtRenameName, renameUserNamesContainingAt); err != nil {
		return err
	}
	return runDataMigrationOnce(db, r2ResolvedFriendNoticesName, deleteResolvedFriendRequestNotices)
}

// deleteResolvedFriendRequestNotices (A-14): xoá thông báo "đã gửi lời mời kết bạn" của các lời mời đã bị
// huỷ/từ chối/chấp nhận/chặn từ trước bản sửa (bấm vào dẫn tới tab Lời mời trống). Dùng chung điều kiện với
// NotificationRepository.DeleteResolvedFriendRequestNotices (model.ResolvedFriendRequestNoticeSQL).
func deleteResolvedFriendRequestNotices(tx *gorm.DB) error {
	return tx.Exec(`DELETE FROM notifications WHERE ` + model.ResolvedFriendRequestNoticeSQL).Error
}

// runDataMigrationOnce chạy fn đúng một lần trên mỗi DB: giao dịch giữ khoá advisory, kiểm bản ghi đánh dấu SAU
// khi giữ khoá (hai tiến trình khởi động song song không cùng chạy), ghi dấu trong cùng giao dịch (lỗi giữa chừng
// thì rollback cả hai, chạy lại được).
func runDataMigrationOnce(db *gorm.DB, name string, fn func(tx *gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", r2CleanupLockKey).Error; err != nil {
			return fmt.Errorf("lấy khoá bước sửa %s: %w", name, err)
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS data_migrations (
			name text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now())`).Error; err != nil {
			return fmt.Errorf("tạo bảng data_migrations: %w", err)
		}
		var done int64
		if err := tx.Raw("SELECT count(*) FROM data_migrations WHERE name = ?", name).Scan(&done).Error; err != nil {
			return fmt.Errorf("đọc data_migrations: %w", err)
		}
		if done > 0 {
			return nil
		}
		if err := fn(tx); err != nil {
			return fmt.Errorf("bước sửa %s: %w", name, err)
		}
		if err := tx.Exec("INSERT INTO data_migrations (name) VALUES (?)", name).Error; err != nil {
			return fmt.Errorf("ghi dấu bước sửa %s: %w", name, err)
		}
		return nil
	})
}

// orphanGroupConversationSQL: hội thoại GROUP còn sống mà nhóm của nó đã xoá mềm hoặc không còn dòng nào.
const orphanGroupConversationSQL = `c.type = 'GROUP' AND c.group_id IS NOT NULL AND c.deleted_at IS NULL
	AND NOT EXISTS (SELECT 1 FROM groups g WHERE g.id = c.group_id AND g.deleted_at IS NULL)`

// closeOrphanGroupConversations: cùng hiệu ứng với GroupRepository.DeleteWithConversation — participant còn hiệu
// lực được đặt left_at (mọi đường đọc/gửi lọc theo cột này) rồi hội thoại bị xoá mềm. Lịch sử tin giữ nguyên.
func closeOrphanGroupConversations(tx *gorm.DB) error {
	if err := tx.Exec(`UPDATE conversation_participants cp SET left_at = NOW(), unread_count = 0
		WHERE cp.left_at IS NULL AND cp.conversation_id IN (SELECT c.id FROM conversations c WHERE ` + orphanGroupConversationSQL + `)`).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE conversations c SET deleted_at = NOW() WHERE ` + orphanGroupConversationSQL).Error
}

type r2NameRow struct {
	ID       string
	FullName *string
}

// renameUserNamesContainingAt đổi mọi user_name có '@' (cả tài khoản đã xoá mềm: tên vẫn có thể hiện ở dữ liệu cũ).
// Tên mới tránh mọi user_name đang dùng (không phân biệt hoa thường) và tên vừa sinh trong cùng lượt. Không đụng
// updated_at: đây là sửa dữ liệu hệ thống. Điều kiện '@' được kiểm lại trong UPDATE để dòng vừa được đổi bởi tiến
// trình khác không bị ghi đè.
func renameUserNamesContainingAt(tx *gorm.DB) error {
	var rows []r2NameRow
	if err := tx.Raw(`SELECT id::text AS id, full_name FROM users WHERE position('@' in user_name) > 0`).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	var used []string
	if err := tx.Raw(`SELECT lower(user_name) FROM users`).Scan(&used).Error; err != nil {
		return err
	}
	taken := make(map[string]struct{}, len(used)+len(rows))
	for _, n := range used {
		taken[n] = struct{}{}
	}
	type rename struct{ id, name string }
	renames := make([]rename, 0, len(rows))
	for _, row := range rows {
		name, err := newFreeUserName(row.FullName, taken, utils.NewSafeUserName)
		if err != nil {
			return fmt.Errorf("user %s: %w", row.ID, err)
		}
		taken[strings.ToLower(name)] = struct{}{}
		renames = append(renames, rename{row.ID, name})
	}
	for start := 0; start < len(renames); start += r2RenameBatch {
		end := min(start+r2RenameBatch, len(renames))
		var values []string
		var args []any
		for _, r := range renames[start:end] {
			values = append(values, "(?::uuid, ?)")
			args = append(args, r.id, r.name)
		}
		sql := `UPDATE users u SET user_name = v.name FROM (VALUES ` + strings.Join(values, ",") +
			`) AS v(id, name) WHERE u.id = v.id AND position('@' in u.user_name) > 0`
		if err := tx.Exec(sql, args...).Error; err != nil {
			return err
		}
	}
	return nil
}
