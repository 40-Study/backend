package database

// google_user_name_backfill.go — đổi user_name cũ của tài khoản Google đang trùng phần trước '@' của email
// (issue #105: lộ một phần email ở leaderboard, contest, danh sách thành viên nhóm).
//
// Chỉ đổi tài khoản được TẠO bằng Google (bản ghi liên kết google ra đời ngay khi tài khoản ra đời, trong
// googleCreatedWindow) và user_name vẫn còn đúng bằng prefix email (tức chưa tự đổi). Người đăng ký bằng
// mật khẩu rồi mới liên kết Google sau đó tự đặt user_name từ đầu nên không bị đụng, kể cả khi tên họ
// tình cờ trùng prefix.
//
// Idempotent: sau khi đổi, user_name không còn trùng prefix nên lần chạy sau (mỗi lần API khởi động)
// không chọn thêm dòng nào; điều kiện được kiểm lại ngay trong câu UPDATE nên người vừa tự đổi tên giữa
// lúc quét và lúc ghi không bị ghi đè.

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"study.com/v1/internal/utils"
)

const googleCreatedWindow = time.Minute

const (
	googleUserNameMatchSQL = `lower(u.user_name) = lower(split_part(u.email, '@', 1))
		AND EXISTS (SELECT 1 FROM user_oauth_providers op
			WHERE op.user_id = u.id AND op.provider = 'google'
			AND op.created_at <= u.created_at + make_interval(secs => ?))`
	googleUserNameMaxRetries = 8
)

type googleNameRow struct {
	ID       string
	FullName *string
}

// runGoogleUserNameBackfill gọi từ RunPostMigrations sau AutoMigrate.
func runGoogleUserNameBackfill(db *gorm.DB) error {
	windowSecs := googleCreatedWindow.Seconds()
	var rows []googleNameRow
	if err := db.Raw(`SELECT u.id::text AS id, u.full_name FROM users u WHERE `+googleUserNameMatchSQL, windowSecs).
		Scan(&rows).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "scan google email-prefix user_name", err)
	}
	for _, row := range rows {
		if err := renameGoogleUser(db, row, windowSecs); err != nil {
			return fmt.Errorf("post-migration %q failed: %w", "rename google email-prefix user_name", err)
		}
	}
	return nil
}

func renameGoogleUser(db *gorm.DB, row googleNameRow, windowSecs float64) error {
	for i := 0; i < googleUserNameMaxRetries; i++ {
		name, err := utils.NewSafeUserName(row.FullName)
		if err != nil {
			return err
		}
		var taken int64
		if err := db.Raw(`SELECT count(*) FROM users WHERE lower(user_name) = lower(?)`, name).Scan(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			continue
		}
		// Không đụng updated_at: đây là sửa dữ liệu hệ thống, không phải người dùng sửa hồ sơ.
		return db.Exec(`UPDATE users u SET user_name = ? WHERE u.id = ?::uuid AND `+googleUserNameMatchSQL,
			name, row.ID, windowSecs).Error
	}
	return fmt.Errorf("không sinh được user_name duy nhất cho user %s", row.ID)
}
