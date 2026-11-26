package database

// user_email_ci_unique.go — QA8 PG-01 (C18). Email tài khoản duy nhất theo CHỮ (idx_users_email trên `email`), nên
// "Parent1@Demo.com" và "parent1@demo.com" là hai tài khoản: giả mạo danh tính và đặt lại mật khẩu nhầm chủ. Mọi đường ghi/tra
// email nay chuẩn hoá chữ thường (utils.NormalizeEmail, repository.UserRepository), index duy nhất trên lower(email) đóng khe
// đó ở tầng DB (đăng ký đồng thời hai kiểu hoa/thường). GORM không khai báo được index biểu thức nên tạo ở đây, cùng mẫu
// system_role_name_ci_unique.go.
//
// An toàn với dữ liệu cũ: index phủ mọi dòng (cả tài khoản đã khoá/xoá mềm). TRƯỚC khi tạo, dò các nhóm email trùng nhau theo
// lower(email); còn nhóm nào thì trả lỗi liệt kê email + id từng dòng và DỪNG (API không khởi động), KHÔNG tự đổi hay xoá dòng
// nào: tài khoản gắn với đơn hàng, khoá học, điểm; chọn tài khoản nào giữ là quyết định của người vận hành. Idempotent.

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const userEmailCIIndex = "uq_users_email_ci"

const userEmailCIDuplicatesSQL = `
	SELECT lower(email) AS folded, string_agg(email || ' (id=' || id::text || ')', ', ' ORDER BY created_at, id) AS rows
	FROM users
	GROUP BY lower(email)
	HAVING count(*) > 1
	ORDER BY lower(email);
`

const userEmailCIIndexSQL = `CREATE UNIQUE INDEX IF NOT EXISTS ` + userEmailCIIndex + ` ON users (lower(email))`

type userEmailDuplicate struct {
	Folded string
	Rows   string
}

// runUserEmailCIUnique gọi từ RunPostMigrations sau AutoMigrate (cần bảng users).
func runUserEmailCIUnique(db *gorm.DB) error {
	var dups []userEmailDuplicate
	if err := db.Raw(userEmailCIDuplicatesSQL).Scan(&dups).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "detect duplicate users emails", err)
	}
	if len(dups) > 0 {
		groups := make([]string, len(dups))
		for i, d := range dups {
			groups[i] = fmt.Sprintf("%q: %s", d.Folded, d.Rows)
		}
		return fmt.Errorf("post-migration %q refused: %d group(s) of users emails collide case-insensitively (soft-deleted rows included), "+
			"so unique index %s cannot be created. No row was modified or deleted. Decide which account of each group to keep, "+
			"then change the email of the others (UPDATE users SET email = '<new email>' WHERE id = '<id>') or merge them by hand; "+
			"accounts are referenced by orders, enrollments and points, so never delete one blindly. Then restart. Colliding groups: %s",
			"unique index on lower(users.email)", len(dups), userEmailCIIndex, strings.Join(groups, "; "))
	}
	if err := db.Exec(userEmailCIIndexSQL).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "create "+userEmailCIIndex, err)
	}
	return nil
}
