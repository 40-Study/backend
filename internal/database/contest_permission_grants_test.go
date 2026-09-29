package database

// Test cho contest_permission_grants.go (Lane G, role-based UX QA 260927): migration ADD-ONLY
// tạo 2 permission cuộc thi + gán role, KHÔNG gọi seeds.SeedAll, KHÔNG xoá/sửa quyền đang có.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

// backendRoot tìm thư mục gốc backend/ (chứa data/) từ vị trí file test này
// (internal/database -> lên 2 cấp), không phụ thuộc working directory lúc `go test` chạy.
func backendRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("không lấy được đường dẫn file test")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

type jsonPermEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type jsonRoleEntry struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

func readPermDescFromJSON(t *testing.T, file, name string) string {
	t.Helper()
	path := filepath.Join(backendRoot(t), "data", "permissions", file)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc %s: %v", path, err)
	}
	var entries []jsonPermEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, e := range entries {
		if e.Name == name {
			return e.Description
		}
	}
	t.Fatalf("permission %s không có trong %s — SSOT (data/permissions/*.json) đã đổi, cập nhật lại contest_permission_grants.go", name, path)
	return ""
}

func roleHasPermissionInJSON(t *testing.T, role, perm string) bool {
	t.Helper()
	path := filepath.Join(backendRoot(t), "data", "roles.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc %s: %v", path, err)
	}
	var roles []jsonRoleEntry
	if err := json.Unmarshal(raw, &roles); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, r := range roles {
		if r.Role != role {
			continue
		}
		if len(r.Permissions) == 1 && r.Permissions[0] == "*" {
			return true // SeedRoles coi "*" là mọi permission hiện có, xem seeds/seeder.go SeedRoles
		}
		for _, p := range r.Permissions {
			if p == perm {
				return true
			}
		}
		return false
	}
	t.Fatalf("role %s không có trong %s", role, path)
	return false
}

// TestContestPermissionGrants_KhopSSOTJson — khoá lệch: hằng số Go trong
// contest_permission_grants.go phải khớp byte-for-byte với data/permissions/*.json +
// data/roles.json (SSOT hiện có của dự án). Sửa mô tả ở JSON mà quên sửa ở đây (hoặc ngược lại)
// sẽ làm test này ĐỎ.
func TestContestPermissionGrants_KhopSSOTJson(t *testing.T) {
	if got := readPermDescFromJSON(t, "teacher_permissions.json", permContestsManageOwn); got != descContestsManageOwn {
		t.Errorf("mô tả %s trong JSON = %q, hằng số Go = %q", permContestsManageOwn, got, descContestsManageOwn)
	}
	if got := readPermDescFromJSON(t, "system_admin_permissions.json", permContestsApproveAll); got != descContestsApproveAll {
		t.Errorf("mô tả %s trong JSON = %q, hằng số Go = %q", permContestsApproveAll, got, descContestsApproveAll)
	}
	if !roleHasPermissionInJSON(t, sysRoleTeacher, permContestsManageOwn) {
		t.Errorf("roles.json: %s không còn liệt kê %s", sysRoleTeacher, permContestsManageOwn)
	}
	if !roleHasPermissionInJSON(t, sysRoleSystemAdmin, permContestsManageOwn) {
		t.Errorf("roles.json: %s không còn bao gồm %s (qua \"*\" hoặc liệt kê tường minh)", sysRoleSystemAdmin, permContestsManageOwn)
	}
	if !roleHasPermissionInJSON(t, sysRoleSystemAdmin, permContestsApproveAll) {
		t.Errorf("roles.json: %s không còn bao gồm %s (qua \"*\" hoặc liệt kê tường minh)", sysRoleSystemAdmin, permContestsApproveAll)
	}
}

// TestContestPermissionGrants_DBTrong_TaoDuQuyen — chạy trên DB trống (chỉ AutoMigrate, chưa
// seed gì): RunPostMigrations phải tạo đủ 2 permission cuộc thi với đúng mô tả. Comment/xoá lời
// gọi contestPermissionPostMigrations() khỏi RunPostMigrations sẽ làm test này ĐỎ (đã tự kiểm 1
// lần, xem báo cáo bàn giao).
func TestContestPermissionGrants_DBTrong_TaoDuQuyen(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations trên DB trống: %v", err)
	}

	for _, tc := range []struct{ name, wantDesc string }{
		{permContestsManageOwn, descContestsManageOwn},
		{permContestsApproveAll, descContestsApproveAll},
	} {
		var p model.Permission
		if err := db.Where("name = ?", tc.name).First(&p).Error; err != nil {
			t.Fatalf("permission %s: %v", tc.name, err)
		}
		if !p.Description.Valid || p.Description.String != tc.wantDesc {
			t.Errorf("permission %s: mô tả = %q, muốn %q", tc.name, p.Description.String, tc.wantDesc)
		}
		if p.Status != "active" {
			t.Errorf("permission %s: status = %q, muốn active", tc.name, p.Status)
		}
	}
}

// TestContestPermissionGrants_ChayHaiLan_KhongTaoTrung — idempotent: chạy RunPostMigrations 2
// lần liên tiếp trên cùng schema không tạo permission trùng tên (unique index name đã chặn ở tầng
// DB, nhưng ON CONFLICT DO NOTHING phải khiến câu lệnh không LỖI ở lần 2).
func TestContestPermissionGrants_ChayHaiLan_KhongTaoTrung(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	for run := 1; run <= 2; run++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("lần %d: %v", run, err)
		}
	}
	var count int64
	if err := db.Model(&model.Permission{}).
		Where("name IN ?", []string{permContestsManageOwn, permContestsApproveAll}).
		Count(&count).Error; err != nil {
		t.Fatalf("đếm permission: %v", err)
	}
	if count != 2 {
		t.Errorf("chạy 2 lần: có %d dòng permission cuộc thi, muốn đúng 2 (không trùng)", count)
	}
}

// TestContestPermissionGrants_GanRoleKhiRoleDaTonTai — khi system_roles đã có SYSTEM_ADMIN và
// TEACHER (mô phỏng lần khởi động THỨ HAI trở đi, sau khi seeds.SeedAll đã tạo role ở lần đầu),
// RunPostMigrations phải gán SYSTEM_ADMIN cả 2 quyền, TEACHER chỉ CONTESTS_MANAGE_OWN — không gán
// CONTESTS_APPROVE_ALL cho TEACHER.
func TestContestPermissionGrants_GanRoleKhiRoleDaTonTai(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	admin := model.SystemRole{Name: sysRoleSystemAdmin, Status: "active"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("tạo role SYSTEM_ADMIN: %v", err)
	}
	teacher := model.SystemRole{Name: sysRoleTeacher, Status: "active"}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatalf("tạo role TEACHER: %v", err)
	}

	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}

	hasPerm := func(roleID interface{ String() string }, permName string) bool {
		var count int64
		db.Table("system_role_permissions").
			Joins("JOIN permissions ON permissions.id = system_role_permissions.permission_id").
			Where("system_role_permissions.system_role_id = ? AND permissions.name = ?", roleID, permName).
			Count(&count)
		return count > 0
	}

	if !hasPerm(admin.ID, permContestsManageOwn) {
		t.Error("SYSTEM_ADMIN thiếu CONTESTS_MANAGE_OWN")
	}
	if !hasPerm(admin.ID, permContestsApproveAll) {
		t.Error("SYSTEM_ADMIN thiếu CONTESTS_APPROVE_ALL")
	}
	if !hasPerm(teacher.ID, permContestsManageOwn) {
		t.Error("TEACHER thiếu CONTESTS_MANAGE_OWN")
	}
	if hasPerm(teacher.ID, permContestsApproveAll) {
		t.Error("TEACHER KHÔNG được có CONTESTS_APPROVE_ALL nhưng migration đã gán")
	}
}

// TestContestPermissionGrants_KhongDungToiQuyenDaGoKhoiRoleKhac — mô phỏng: admin đã gán 1
// permission khác cho ORG_OWNER rồi TỰ TAY gỡ (xoá dòng system_role_permissions). Chạy
// RunPostMigrations xong, dòng đã gỡ đó KHÔNG được phục hồi — chứng minh migration lane G chỉ
// đụng đúng 3 cặp (role, permission) của nó, không "quét lại" toàn bộ role_permissions như
// seeds.SeedRoles.
func TestContestPermissionGrants_KhongDungToiQuyenDaGoKhoiRoleKhac(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)

	orgOwner := model.SystemRole{Name: "ORG_OWNER", Status: "active"}
	if err := db.Create(&orgOwner).Error; err != nil {
		t.Fatalf("tạo role ORG_OWNER: %v", err)
	}
	other := model.Permission{Name: "QA_OTHER_PERM_260929"}
	other.Description.String, other.Description.Valid = "Quyền khác dùng để test cô lập", true
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("tạo permission khác: %v", err)
	}
	link := model.SystemRolePermission{SystemRoleID: orgOwner.ID, PermissionID: other.ID}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("gán quyền khác cho ORG_OWNER: %v", err)
	}
	// Admin tự tay gỡ quyền này khỏi ORG_OWNER.
	if err := db.Delete(&model.SystemRolePermission{}, "system_role_id = ? AND permission_id = ?", orgOwner.ID, other.ID).Error; err != nil {
		t.Fatalf("gỡ quyền khác: %v", err)
	}

	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}

	var count int64
	db.Table("system_role_permissions").
		Where("system_role_id = ? AND permission_id = ?", orgOwner.ID, other.ID).
		Count(&count)
	if count != 0 {
		t.Errorf("quyền QA_OTHER_PERM_260929 đã gỡ khỏi ORG_OWNER nhưng bị migration lane G phục hồi lại (count=%d)", count)
	}
}
