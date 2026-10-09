package seeds

// Re-seed chạy MỖI lần API khởi động (SeedAll). SeedPermissions dùng FirstOrCreate nên trước đây
// quyền đã có KHÔNG BAO GIỜ được cập nhật mô tả: sửa chữ trong data/permissions/*.json không tới
// được DB đang chạy. Test chạy trên Postgres THẬT trong schema tạm (DROP khi xong); đưa seeder về
// "chỉ FirstOrCreate" thì ĐỎ.

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func writePermissionSeed(t *testing.T, items ...PermissionSeed) string {
	t.Helper()
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "perms.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func permissionByName(t *testing.T, db *gorm.DB, name string) model.Permission {
	t.Helper()
	var p model.Permission
	if err := db.Where("name = ?", name).First(&p).Error; err != nil {
		t.Fatalf("đọc quyền %s: %v", name, err)
	}
	return p
}

func TestPermissionDescriptionResync_ExistingRowGetsNewDescription(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	const name = "QA_RESYNC_PERMISSION"
	old := model.Permission{Name: name, Status: "inactive", Description: sql.NullString{String: "Mô tả cũ (A-P0-1, QA 260927)", Valid: true}}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	role := model.SystemRole{Name: "QA_RESYNC_ROLE", Status: "active"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SystemRolePermission{SystemRoleID: role.ID, PermissionID: old.ID}).Error; err != nil {
		t.Fatal(err)
	}

	path := writePermissionSeed(t, PermissionSeed{Name: name, Description: "Mô tả mới sạch"})
	if err := NewSeeder(db).SeedPermissions(path); err != nil {
		t.Fatalf("SeedPermissions: %v", err)
	}

	got := permissionByName(t, db, name)
	if got.Description.String != "Mô tả mới sạch" || !got.Description.Valid {
		t.Fatalf("mô tả trong DB = %+v, muốn %q (re-seed không cập nhật mô tả quyền đã có)", got.Description, "Mô tả mới sạch")
	}
	// S2 (additive): chỉ mô tả đổi. id, status (admin có thể đã tắt) và gán quyền cho role giữ nguyên.
	if got.ID != old.ID || got.Status != "inactive" {
		t.Errorf("re-seed đụng vào id/status: %+v (id cũ %s)", got, old.ID)
	}
	var assigned int64
	db.Model(&model.SystemRolePermission{}).Where("system_role_id = ? AND permission_id = ?", role.ID, old.ID).Count(&assigned)
	if assigned != 1 {
		t.Errorf("gán quyền cho role bị đổi: %d dòng, muốn 1", assigned)
	}
	var total int64
	db.Model(&model.Permission{}).Where("name = ?", name).Count(&total)
	if total != 1 {
		t.Errorf("re-seed tạo trùng quyền: %d dòng", total)
	}
}

func TestPermissionDescriptionResync_UnchangedDescriptionIsNotRewritten(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	path := writePermissionSeed(t, PermissionSeed{Name: "QA_RESYNC_STABLE", Description: "Mô tả ổn định"})
	s := NewSeeder(db)
	if err := s.SeedPermissions(path); err != nil { // lần đầu: tạo mới
		t.Fatal(err)
	}
	first := permissionByName(t, db, "QA_RESYNC_STABLE")
	if first.Description.String != "Mô tả ổn định" {
		t.Fatalf("quyền mới tạo sai mô tả: %+v", first.Description)
	}
	if err := s.SeedPermissions(path); err != nil { // khởi động lại: không có gì đổi
		t.Fatal(err)
	}
	second := permissionByName(t, db, "QA_RESYNC_STABLE")
	if !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Errorf("mô tả không đổi mà vẫn ghi lại dòng: updated_at %v -> %v", first.UpdatedAt, second.UpdatedAt)
	}
}

func TestPermissionDescriptionResync_SeedAllRewritesTaggedRowsFromRealJSON(t *testing.T) {
	db, s := s2SeededDB(t)
	// Mô phỏng DB đã deploy trước khi dọn tag: ghi lại mô tả cũ rồi khởi động lại.
	if err := db.Exec(`UPDATE permissions SET description = description || ' (Phase 4, 28/09/2026)'
		WHERE name = 'WALLET_WITHDRAWALS_MANAGE'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.SeedAll(s2DataDir); err != nil {
		t.Fatalf("SeedAll lần 2: %v", err)
	}
	got := permissionByName(t, db, "WALLET_WITHDRAWALS_MANAGE")
	if got.Description.String == "" || tagPattern.MatchString(got.Description.String) {
		t.Fatalf("khởi động lại phải ghi đè mô tả có tag, nhận %q", got.Description.String)
	}
}
