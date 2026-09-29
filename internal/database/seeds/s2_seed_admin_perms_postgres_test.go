package seeds

// Lane S2, lỗi 5: SeedAll chạy MỖI lần API khởi động. Trước đây nó xoá toàn bộ
// system_role_permissions của role rồi chèn lại từ roles.json (không transaction), nên quyền admin
// vừa gỡ tay lại hiện về sau lần khởi động kế tiếp và role có lúc trống quyền. Các test dưới đây chạy
// trên Postgres THẬT trong schema tạm (DROP khi xong); revert về "xoá rồi chèn lại" thì ĐỎ.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

const s2DataDir = "../../../data" // internal/database/seeds -> thư mục gốc backend

func s2SeededDB(t *testing.T) (*gorm.DB, *Seeder) {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	if err := s.SeedAll(s2DataDir); err != nil {
		t.Fatalf("SeedAll lần đầu: %v", err)
	}
	return db, s
}

// s2RolePerms trả tên quyền hiện có của role.
func s2RolePerms(t *testing.T, db *gorm.DB, role string) map[string]bool {
	t.Helper()
	var names []string
	err := db.Table("system_role_permissions AS srp").
		Joins("JOIN system_roles sr ON sr.id = srp.system_role_id").
		Joins("JOIN permissions p ON p.id = srp.permission_id").
		Where("sr.name = ?", role).Pluck("p.name", &names).Error
	if err != nil {
		t.Fatalf("đọc quyền role %s: %v", role, err)
	}
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}

// adminRemove mô phỏng đúng đường quản trị (SystemRoleRepository.SetSystemRolePermissions): xoá
// dòng system_role_permissions, KHÔNG đụng bảng dấu của seeder.
func s2AdminRemove(t *testing.T, db *gorm.DB, role, perm string) {
	t.Helper()
	res := db.Exec(`DELETE FROM system_role_permissions WHERE system_role_id = (SELECT id FROM system_roles WHERE name = ?)
		AND permission_id = (SELECT id FROM permissions WHERE name = ?)`, role, perm)
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("gỡ quyền %s khỏi %s: rows=%d err=%v", perm, role, res.RowsAffected, res.Error)
	}
}

func s2AnyPerm(t *testing.T, perms map[string]bool) string {
	t.Helper()
	for p := range perms {
		return p
	}
	t.Fatal("role không có quyền nào để gỡ")
	return ""
}

func TestS2_SeedAll_QuyenAdminDaGoKhongBiCapLai(t *testing.T) {
	db, s := s2SeededDB(t)

	for _, role := range []string{"TEACHER", "SYSTEM_ADMIN"} { // SYSTEM_ADMIN dùng "*"
		before := s2RolePerms(t, db, role)
		removed := s2AnyPerm(t, before)
		s2AdminRemove(t, db, role, removed)

		if err := s.SeedAll(s2DataDir); err != nil {
			t.Fatalf("SeedAll lần 2: %v", err)
		}
		after := s2RolePerms(t, db, role)
		if after[removed] {
			t.Errorf("role %s: quyền %s admin đã gỡ bị seed cấp lại sau khi khởi động", role, removed)
		}
		if len(after) != len(before)-1 {
			t.Errorf("role %s: số quyền %d -> %d, muốn %d (chỉ mất đúng quyền đã gỡ)", role, len(before), len(after), len(before)-1)
		}
	}
}

// Seed vẫn phải TỚI được các quyền mới: cặp (role, quyền) chưa từng được seed cấp thì lần chạy
// sau cấp (đây là lý do cần bảng dấu thay vì "không bao giờ chèn lại").
func TestS2_SeedAll_VanCapCapQuyenChuaTungSeed(t *testing.T) {
	db, s := s2SeededDB(t)
	before := s2RolePerms(t, db, "TEACHER")
	perm := s2AnyPerm(t, before)

	s2AdminRemove(t, db, "TEACHER", perm)
	if err := db.Exec(`DELETE FROM system_role_permission_seeds WHERE system_role_id = (SELECT id FROM system_roles WHERE name = 'TEACHER')
		AND permission_id = (SELECT id FROM permissions WHERE name = ?)`, perm).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.SeedAll(s2DataDir); err != nil {
		t.Fatal(err)
	}
	if !s2RolePerms(t, db, "TEACHER")[perm] {
		t.Fatalf("cặp (TEACHER, %s) chưa từng được seed cấp phải được cấp ở lần chạy sau", perm)
	}
}

// Quyền admin CẤP THÊM tay (không có trong roles.json) và trạng thái role admin đã đổi phải sống
// qua khởi động lại — trước đây bị xoá sạch / ép về active.
func TestS2_SeedAll_GiuQuyenCapThemVaTrangThaiRole(t *testing.T) {
	db, s := s2SeededDB(t)

	extra := model.Permission{Name: "S2_EXTRA_PERMISSION_DO_ADMIN_CAP", Status: "active"}
	if err := db.Create(&extra).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO system_role_permissions (system_role_id, permission_id, created_at)
		SELECT id, ?, now() FROM system_roles WHERE name = 'TEACHER'`, extra.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE system_roles SET status = 'inactive' WHERE name = 'TEACHER'`).Error; err != nil {
		t.Fatal(err)
	}

	if err := s.SeedAll(s2DataDir); err != nil {
		t.Fatal(err)
	}
	if !s2RolePerms(t, db, "TEACHER")[extra.Name] {
		t.Error("quyền admin cấp thêm cho TEACHER bị seed xoá")
	}
	var status string
	if err := db.Raw(`SELECT status FROM system_roles WHERE name = 'TEACHER'`).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "inactive" {
		t.Errorf("role TEACHER admin đã tắt bị seed ép về %q", status)
	}
}

// SeedAll phải là MỘT transaction: một file quyền lỗi ở giữa thì toàn bộ seed rollback, không để lại
// quyền/role thấy một nửa. Tên quyền 150 ký tự vượt varchar(100) -> Postgres từ chối.
func TestS2_SeedAll_NguyenTuKhiMotFileLoi(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "permissions"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel string, v interface{}) {
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, rel), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// ReadDir trả theo thứ tự tên: a_ok chạy trước b_bad.
	write("permissions/a_ok.json", []PermissionSeed{{Name: "S2_ATOMIC_OK"}})
	write("permissions/b_bad.json", []PermissionSeed{{Name: "S2_ATOMIC_" + strings.Repeat("x", 150)}})
	write("roles.json", []RoleSeed{})

	if err := NewSeeder(db).SeedAll(dir); err == nil {
		t.Fatal("SeedAll phải trả lỗi khi một file quyền không ghi được")
	}
	var n int64
	if err := db.Model(&model.Permission{}).Where("name = ?", "S2_ATOMIC_OK").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("quyền của file đầu vẫn còn (%d) dù seed lỗi ở file sau: SeedAll không nằm trong transaction", n)
	}
}
