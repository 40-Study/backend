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
// không chọn thêm dòng nào. Điều kiện được kiểm lại ngay trong câu UPDATE (applyGoogleRenames) nên người vừa
// tự đổi tên giữa lúc quét và lúc ghi, hoặc hai instance khởi động cùng lúc, không bị ghi đè.
//
// Hiệu năng: RunPostMigrations chạy TRƯỚC khi API listen nên mọi giây ở đây là thời gian khởi động. Cột user_name
// không có index và lower() vô hiệu hoá index thường, nên kiểm trùng bằng một câu SELECT cho MỖI dòng là R x N
// (đo trên 505k user, 5k dòng cần đổi: 13 phút). Nay nạp tập tên đang dùng vào bộ nhớ MỘT lần rồi kiểm trùng
// trong map, và UPDATE theo lô bằng một câu cho mỗi googleRenameBatch dòng.

import (
	"fmt"
	"strings"
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
	googleRenameBatch        = 500
)

type googleNameRow struct {
	ID       string
	FullName *string
}

type googleRename struct{ ID, Name string }

// runGoogleUserNameBackfill gọi từ RunPostMigrations sau AutoMigrate.
func runGoogleUserNameBackfill(db *gorm.DB) error {
	windowSecs := googleCreatedWindow.Seconds()
	rows, err := scanGoogleEmailPrefixUsers(db, windowSecs)
	if err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "scan google email-prefix user_name", err)
	}
	if len(rows) == 0 {
		return nil // đường thường gặp (mỗi lần khởi động sau lần đầu): không nạp tập tên
	}
	renames, err := planGoogleRenames(db, rows)
	if err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "plan google user_name renames", err)
	}
	if err := applyGoogleRenames(db, renames, windowSecs); err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "rename google email-prefix user_name", err)
	}
	return nil
}

func scanGoogleEmailPrefixUsers(db *gorm.DB, windowSecs float64) ([]googleNameRow, error) {
	var rows []googleNameRow
	err := db.Raw(`SELECT u.id::text AS id, u.full_name FROM users u WHERE `+googleUserNameMatchSQL, windowSecs).Scan(&rows).Error
	return rows, err
}

// planGoogleRenames sinh tên mới cho từng dòng, tránh mọi user_name đang dùng (không phân biệt hoa thường, cả
// tài khoản đã xoá mềm) và tránh các tên vừa sinh trong cùng lượt. Tập tên đang dùng nạp vào map một lần.
func planGoogleRenames(db *gorm.DB, rows []googleNameRow) ([]googleRename, error) {
	var used []string
	if err := db.Raw(`SELECT lower(user_name) FROM users`).Scan(&used).Error; err != nil {
		return nil, err
	}
	taken := make(map[string]struct{}, len(used)+len(rows))
	for _, n := range used {
		taken[n] = struct{}{}
	}
	out := make([]googleRename, 0, len(rows))
	for _, row := range rows {
		name, err := newFreeUserName(row.FullName, taken, utils.NewSafeUserName)
		if err != nil {
			return nil, fmt.Errorf("user %s: %w", row.ID, err)
		}
		taken[strings.ToLower(name)] = struct{}{}
		out = append(out, googleRename{ID: row.ID, Name: name})
	}
	return out, nil
}

// newFreeUserName gọi gen tới khi ra tên chưa nằm trong taken (tối đa googleUserNameMaxRetries lần).
func newFreeUserName(fullName *string, taken map[string]struct{}, gen func(*string) (string, error)) (string, error) {
	for i := 0; i < googleUserNameMaxRetries; i++ {
		name, err := gen(fullName)
		if err != nil {
			return "", err
		}
		if _, dup := taken[strings.ToLower(name)]; !dup {
			return name, nil
		}
	}
	return "", fmt.Errorf("không sinh được user_name duy nhất sau %d lần", googleUserNameMaxRetries)
}

// applyGoogleRenames ghi theo lô. Mỗi câu UPDATE vẫn kiểm lại googleUserNameMatchSQL nên dòng đã bị đổi/tự
// đổi tên kể từ lúc quét thì KHÔNG bị ghi đè. Không đụng updated_at: đây là sửa dữ liệu hệ thống.
func applyGoogleRenames(db *gorm.DB, renames []googleRename, windowSecs float64) error {
	for start := 0; start < len(renames); start += googleRenameBatch {
		end := min(start+googleRenameBatch, len(renames))
		var values []string
		var args []any
		for _, r := range renames[start:end] {
			values = append(values, "(?::uuid, ?)")
			args = append(args, r.ID, r.Name)
		}
		args = append(args, windowSecs)
		sql := `UPDATE users u SET user_name = v.name FROM (VALUES ` + strings.Join(values, ",") +
			`) AS v(id, name) WHERE u.id = v.id AND ` + googleUserNameMatchSQL
		if err := db.Exec(sql, args...).Error; err != nil {
			return err
		}
	}
	return nil
}
