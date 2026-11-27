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
	WHERE deleted_at IS NULL
	GROUP BY lower(email)
	HAVING count(*) > 1
	ORDER BY lower(email);
`

// Cùng phạm vi xoá mềm với idx_users_email (H3): user đã xoá mềm không khoá email vĩnh viễn (QA8 review L2).
const userEmailCIIndexSQL = `CREATE UNIQUE INDEX IF NOT EXISTS ` + userEmailCIIndex + ` ON users (lower(email)) WHERE deleted_at IS NULL`

// Bản đầu của index (trước review L2) là index đầy đủ: bỏ đi để tạo lại dạng partial.
// KHÔNG dùng view pg_indexes: nó gọi pg_get_indexdef() trên index của MỌI schema, và planner có thể tính cột indexdef trước
// khi lọc schemaname. Khi gói test khác đang DROP SCHEMA tạm song song, index đó biến mất giữa chừng và câu lệnh chết với
// "could not open relation with OID n" (SQLSTATE XX000). Thay vào đó phân giải đúng OID index của schema hiện tại bằng
// to_regclass (không bao giờ ném lỗi, trả NULL khi chưa có) rồi chỉ gọi pg_get_indexdef trên OID đó. Idempotent: DB mới
// (chưa có index) hoặc index đã partial thì không làm gì.
const dropFullUserEmailCIIndexSQL = `
	DO $$
	DECLARE idx regclass := to_regclass(quote_ident(current_schema()) || '.' || quote_ident('` + userEmailCIIndex + `'));
	BEGIN
		IF idx IS NOT NULL
			AND EXISTS (SELECT 1 FROM pg_index WHERE indexrelid = idx)
			AND pg_get_indexdef(idx) NOT ILIKE '%WHERE%' THEN
			EXECUTE 'DROP INDEX ' || idx::text;
		END IF;
	END $$;
`

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
		return fmt.Errorf("post-migration %q refused: %d group(s) of users emails collide case-insensitively (soft-deleted rows excluded), "+
			"so unique index %s cannot be created. No row was modified or deleted. Decide which account of each group to keep, "+
			"then change the email of the others (UPDATE users SET email = '<new email>' WHERE id = '<id>') or merge them by hand; "+
			"accounts are referenced by orders, enrollments and points, so never delete one blindly. Then restart. Colliding groups: %s",
			"unique index on lower(users.email)", len(dups), userEmailCIIndex, strings.Join(groups, "; "))
	}
	if err := db.Exec(dropFullUserEmailCIIndexSQL).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "drop full "+userEmailCIIndex, err)
	}
	if err := db.Exec(userEmailCIIndexSQL).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "create "+userEmailCIIndex, err)
	}
	return nil
}
