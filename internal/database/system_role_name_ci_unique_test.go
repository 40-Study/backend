package database

// Test Postgres THẬT (schema tạm) cho unique index lower(system_roles.name) (system_role_name_ci_unique.go), gọi qua RunPostMigrations
// như lúc API khởi động. Đỏ khi: bỏ lời gọi khỏi RunPostMigrations (ca index), đổi sang tự xoá/đổi tên dòng trùng (ca dừng, vì
// kiểm số dòng còn lại), bỏ nhóm vai đã xoá mềm khỏi phép dò (ca xoá mềm), hoặc tạo index mà không dò trước (ca trùng không lỗi).

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func roleNameIndexExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`, systemRoleNameCIIndex).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func newRole(t *testing.T, db *gorm.DB, name string, softDeleted bool) uuid.UUID {
	t.Helper()
	r := model.SystemRole{Name: name}
	if err := db.Create(&r).Error; err != nil {
		t.Fatalf("tạo vai %q: %v", name, err)
	}
	if softDeleted {
		if err := db.Delete(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	return r.ID
}

func roleRows(t *testing.T, db *gorm.DB, prefix string) int64 {
	t.Helper()
	var n int64
	if err := db.Unscoped().Model(&model.SystemRole{}).Where("name ILIKE ?", prefix+"%").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSystemRoleNameCIUnique_CreatesIndexAndBlocksCaseVariants_Idempotent(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	for run := 1; run <= 2; run++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("lần %d RunPostMigrations: %v", run, err)
		}
	}
	if !roleNameIndexExists(t, db) {
		t.Fatalf("thiếu index %s sau RunPostMigrations", systemRoleNameCIIndex)
	}
	base := "Qa7b_Moderator_" + uuid.NewString()[:6]
	newRole(t, db, base, false)
	err := db.Create(&model.SystemRole{Name: strings.ToLower(base)}).Error
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != systemRoleNameCIIndex {
		t.Fatalf("tạo biến thể hoa/thường: err=%v, muốn 23505 trên %s", err, systemRoleNameCIIndex)
	}
	// Tên khác hẳn vẫn tạo được.
	newRole(t, db, base+"_other", false)
}

func TestSystemRoleNameCIUnique_DuplicatesStopLoudly_NothingDeleted_ThenCleanRunSucceeds(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	// Dựng lại trạng thái DB cũ: chưa có index theo lower(name), nên hai tên khác hoa/thường cùng tồn tại được.
	if err := db.Exec(`DROP INDEX IF EXISTS ` + systemRoleNameCIIndex).Error; err != nil {
		t.Fatal(err)
	}
	prefix := "Qa7b_Dup_" + uuid.NewString()[:6]
	keep := newRole(t, db, prefix, false)
	dupLive := newRole(t, db, strings.ToUpper(prefix), false)
	other := prefix + "_gone"
	newRole(t, db, other, false)
	dupSoft := newRole(t, db, strings.ToUpper(other), true) // dòng đã xoá mềm cũng giữ chỗ trong index

	err := RunPostMigrations(db)
	if err == nil {
		t.Fatal("còn tên trùng không phân biệt hoa thường mà RunPostMigrations vẫn thành công")
	}
	msg := err.Error()
	for _, want := range []string{systemRoleNameCIIndex, "2 nhóm", strings.ToLower(prefix), keep.String(), dupLive.String(), dupSoft.String(), "Không dòng nào bị sửa hay xoá"} {
		if !strings.Contains(msg, want) {
			t.Errorf("lỗi thiếu %q: %s", want, msg)
		}
	}
	if got := roleRows(t, db, "Qa7b_Dup_"); got != 4 {
		t.Errorf("còn %d dòng, muốn 4: migration không được xoá dòng nào", got)
	}
	if roleNameIndexExists(t, db) {
		t.Error("index không được tạo khi còn tên trùng")
	}

	// Người vận hành xử lý tay (xoá hẳn hai dòng thừa) rồi khởi động lại: thành công, index được tạo.
	if err := db.Unscoped().Delete(&model.SystemRole{}, "id IN ?", []uuid.UUID{dupLive, dupSoft}).Error; err != nil {
		t.Fatal(err)
	}
	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("sau khi dọn trùng: %v", err)
	}
	if !roleNameIndexExists(t, db) {
		t.Fatalf("thiếu index %s sau khi dọn trùng", systemRoleNameCIIndex)
	}
}
