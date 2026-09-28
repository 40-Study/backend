package pgtest

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

// IsolatedSchema phải (1) ghi vào 1 schema tạm t_*, KHÔNG vào public của DB dùng chung, và (2)
// xoá schema đó khi test xong — review Phase 4, B-2 (không để lại dữ liệu/constraint trên study_db).
func TestIsolatedSchema_StaysOutOfPublicAndIsDropped(t *testing.T) {
	var schema string
	t.Run("inner", func(t *testing.T) {
		db := IsolatedSchema(t, func(db *gorm.DB) error {
			return db.Exec("CREATE TABLE pgtest_probe (id int)").Error
		})
		if err := db.Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
			t.Fatalf("current_schema: %v", err)
		}
		if !strings.HasPrefix(schema, "t_") {
			t.Fatalf("current_schema = %q, muốn schema tạm t_*", schema)
		}
		var inPublic *string
		db.Raw("SELECT to_regclass('public.pgtest_probe')::text").Scan(&inPublic)
		if inPublic != nil {
			t.Fatal("bảng của test lọt vào schema public")
		}
	})
	var n int64
	Open(t).Raw("SELECT count(*) FROM information_schema.schemata WHERE schema_name = ?", schema).Scan(&n)
	if n != 0 {
		t.Fatalf("schema tạm %s còn tồn tại sau khi test xong", schema)
	}
}

// Trên CI (CI=true) thiếu DB phải FAIL, không được SKIP (review Phase 4, B-2).
func TestMissingDBAction(t *testing.T) {
	cases := map[string]string{"true": "fail", "TRUE": "fail", " true ": "fail", "": "skip", "false": "skip", "1": "skip"}
	for in, want := range cases {
		if got := MissingDBAction(in); got != want {
			t.Fatalf("MissingDBAction(%q) = %q, muốn %q", in, got, want)
		}
	}
}
