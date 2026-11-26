package database

// system_role_name_ci_unique.go — QA7 backlog (#155, review M4). Tên vai trò hệ thống đã duy nhất theo CHỮ (uniqueIndex trên
// `name`), còn service chỉ kiểm trùng không phân biệt hoa thường bằng một câu SELECT trước khi ghi, nên hai request đồng thời
// "Moderator" / "moderator" vẫn cùng lọt qua. Index duy nhất trên lower(name) đóng hẳn khe đó ở tầng DB (service đã ánh xạ
// mọi vi phạm unique, isUniqueViolation, sang ErrSystemRoleNameExists/409). GORM không khai báo được index biểu thức nên tạo ở đây.
//
// An toàn với dữ liệu cũ: index phủ cả vai đã xoá mềm (giống SystemRoleNameTakenFold, Unscoped). TRƯỚC khi tạo, dò các nhóm
// tên trùng nhau theo lower(name); còn nhóm nào thì trả lỗi nêu rõ tên + id từng dòng và DỪNG (API không khởi động), KHÔNG tự xoá,
// đổi tên hay gộp dòng nào: vai trò đang gắn với user và quyền, chọn dòng nào giữ là quyết định của người vận hành. Nên đổi
// tên các dòng thừa (an toàn hơn xoá hẳn: vai đang được tham chiếu có thể vướng khoá ngoại) rồi khởi động lại; lần sau dò sạch thì tạo index. Idempotent (IF NOT EXISTS).

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const systemRoleNameCIIndex = "uq_system_roles_name_ci"

const systemRoleNameCIDuplicatesSQL = `
	SELECT lower(name) AS folded, string_agg(name || ' (id=' || id::text || ')', ', ' ORDER BY created_at, id) AS rows
	FROM system_roles
	GROUP BY lower(name)
	HAVING count(*) > 1
	ORDER BY lower(name);
`

const systemRoleNameCIIndexSQL = `CREATE UNIQUE INDEX IF NOT EXISTS ` + systemRoleNameCIIndex + ` ON system_roles (lower(name))`

type systemRoleNameDuplicate struct {
	Folded string
	Rows   string
}

// runSystemRoleNameCIUnique gọi từ RunPostMigrations sau AutoMigrate (cần bảng system_roles).
func runSystemRoleNameCIUnique(db *gorm.DB) error {
	var dups []systemRoleNameDuplicate
	if err := db.Raw(systemRoleNameCIDuplicatesSQL).Scan(&dups).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "detect duplicate system_roles names", err)
	}
	if len(dups) > 0 {
		groups := make([]string, len(dups))
		for i, d := range dups {
			groups[i] = fmt.Sprintf("%q: %s", d.Folded, d.Rows)
		}
		return fmt.Errorf("post-migration %q refused: %d group(s) of system_roles names collide case-insensitively (soft-deleted rows included), "+
			"so unique index %s cannot be created. No row was modified or deleted. Rename the extra rows "+
			"(UPDATE system_roles SET name = '<new name>' WHERE id = '<id>'): renaming is safer than deleting, because a role still referenced by "+
			"user_system_roles or system_role_permissions can fail to delete on a foreign key or take its assignments with it. Then restart. Colliding groups: %s",
			"unique index on lower(system_roles.name)", len(dups), systemRoleNameCIIndex, strings.Join(groups, "; "))
	}
	if err := db.Exec(systemRoleNameCIIndexSQL).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "create "+systemRoleNameCIIndex, err)
	}
	return nil
}
