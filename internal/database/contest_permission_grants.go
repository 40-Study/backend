package database

import "fmt"

// Lane G (role-based UX QA, 260927) — đảm bảo 2 permission cuộc thi tồn tại và được gán cho đúng
// role NGAY CẢ KHI seeds.SeedAll (internal/app/app.go, chạy mỗi lần API khởi động) vì lý do nào đó
// không chạy hoặc chạy trước khi các permission này được thêm vào data/permissions/*.json.
//
// Tên + mô tả COPY Y HỆT từ data/permissions/system_admin_permissions.json,
// data/permissions/teacher_permissions.json và data/roles.json — đó là SSOT hiện có của dự án
// (seeds.SeedPermissions/SeedRoles đọc từ các file này). Test
// contest_permission_grants_test.go đọc lại các file JSON đó và so khớp byte-for-byte với các
// hằng số bên dưới, để không ai sửa một bên mà quên bên kia.
const (
	permContestsManageOwn = "CONTESTS_MANAGE_OWN"
	descContestsManageOwn = "Tạo, sửa, gửi duyệt cuộc thi do mình tạo"

	permContestsApproveAll = "CONTESTS_APPROVE_ALL"
	descContestsApproveAll = "Duyệt, từ chối, huỷ cuộc thi, gắn giải voucher và chốt kết quả mọi cuộc thi"

	sysRoleSystemAdmin = "SYSTEM_ADMIN"
	sysRoleTeacher     = "TEACHER"
)

// insertPermissionSQL sinh câu INSERT ... ON CONFLICT (name) DO NOTHING cho một permission — CHỈ
// THÊM khi permissions.name (unique index, xem model.Permission) chưa tồn tại, không bao giờ UPDATE
// permission đã có (khác SeedPermissions trong seeds/seeder.go, hàm đó có cập nhật description mỗi
// lần chạy). name/description luôn là hằng số Go compile-time ở trên, không phải input người dùng,
// nên nối chuỗi trực tiếp vào SQL literal an toàn — cùng khuôn với buildOrderStatusConstraintSQL.
func insertPermissionSQL(name, description string) string {
	return fmt.Sprintf(`
		INSERT INTO permissions (id, name, description, status, created_at, updated_at)
		VALUES (gen_random_uuid(), '%s', '%s', 'active', now(), now())
		ON CONFLICT (name) DO NOTHING;
	`, name, description)
}

// grantSystemRolePermissionSQL sinh câu INSERT ... SELECT ... ON CONFLICT DO NOTHING gán một
// permission cho một system role theo TÊN. Nếu role hoặc permission chưa tồn tại (vd. lần khởi
// động ĐẦU TIÊN trên DB trống, khi RunPostMigrations chạy TRƯỚC seeds.SeedAll nên system_roles
// chưa được tạo — xem internal/app/app.go), SELECT khớp 0 dòng và câu lệnh không làm gì, không lỗi;
// lần khởi động KẾ TIẾP (khi role đã tồn tại) sẽ tự chèn vì RunPostMigrations chạy lại mỗi lần boot.
//
// ON CONFLICT DO NOTHING trên khoá chính (system_role_id, permission_id) — nghĩa là: nếu dòng gán
// này ĐÃ CÓ (do SeedRoles hoặc do chính câu lệnh này chèn ở lần boot trước) thì bỏ qua, KHÔNG BAO
// GIỜ xoá hay sửa dòng nào khác của role đó. Đây là khác biệt cố ý so với seeds.SeedRoles
// (seeds/seeder.go SeedRoles) — hàm đó XOÁ HẾT system_role_permissions của một role rồi CHÈN LẠI
// toàn bộ danh sách permission từ roles.json mỗi lần server khởi động.
//
// GIỚI HẠN đã biết (ghi trong báo cáo bàn giao): nếu admin đã tự tay GỠ CONTESTS_MANAGE_OWN khỏi
// TEACHER (xoá dòng system_role_permissions), câu SQL bên dưới sẽ CHÈN LẠI ở lần khởi động kế tiếp
// — vì ON CONFLICT DO NOTHING chỉ biết "dòng đã có chưa", không phân biệt được "chưa từng chèn" với
// "đã bị gỡ tay". Dự án CHƯA có bảng/cờ đánh dấu "post-migration nào đã chạy rồi" (grep
// migrations_applied/schema_migrations/applied_migrations = 0 kết quả) để phân biệt hai trường hợp
// này; xây thêm cơ chế đó vượt phạm vi yêu cầu (2 permission cuộc thi), nên không tự ý thêm ở đây.
func grantSystemRolePermissionSQL(roleName, permName string) string {
	return fmt.Sprintf(`
		INSERT INTO system_role_permissions (system_role_id, permission_id, created_at)
		SELECT sr.id, p.id, now()
		FROM system_roles sr, permissions p
		WHERE sr.name = '%s' AND p.name = '%s'
		ON CONFLICT (system_role_id, permission_id) DO NOTHING;
	`, roleName, permName)
}

// contestPermissionPostMigrations — nối vào CUỐI danh sách statements trong RunPostMigrations
// (migrations.go), cùng khuôn với contestPostMigrations(). CHỈ THÊM, không gọi seeds.SeedAll.
func contestPermissionPostMigrations() []struct {
	name string
	sql  string
} {
	return []struct {
		name string
		sql  string
	}{
		{
			name: "insert permission CONTESTS_MANAGE_OWN nếu chưa có (lane G, add-only)",
			sql:  insertPermissionSQL(permContestsManageOwn, descContestsManageOwn),
		},
		{
			name: "insert permission CONTESTS_APPROVE_ALL nếu chưa có (lane G, add-only)",
			sql:  insertPermissionSQL(permContestsApproveAll, descContestsApproveAll),
		},
		{
			name: "gán CONTESTS_MANAGE_OWN cho SYSTEM_ADMIN nếu role đã tồn tại (lane G, add-only)",
			sql:  grantSystemRolePermissionSQL(sysRoleSystemAdmin, permContestsManageOwn),
		},
		{
			name: "gán CONTESTS_APPROVE_ALL cho SYSTEM_ADMIN nếu role đã tồn tại (lane G, add-only)",
			sql:  grantSystemRolePermissionSQL(sysRoleSystemAdmin, permContestsApproveAll),
		},
		{
			name: "gán CONTESTS_MANAGE_OWN cho TEACHER nếu role đã tồn tại (lane G, add-only)",
			sql:  grantSystemRolePermissionSQL(sysRoleTeacher, permContestsManageOwn),
		},
	}
}
