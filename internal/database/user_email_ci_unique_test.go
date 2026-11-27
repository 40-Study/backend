package database

// Test Postgres THẬT (schema tạm) cho unique index lower(users.email) (user_email_ci_unique.go), gọi qua RunPostMigrations như
// lúc API khởi động (QA8 PG-01, C18). Đỏ khi: bỏ lời gọi khỏi RunPostMigrations (ca index), đổi sang tự xoá/đổi email dòng trùng
// (ca dừng, kiểm email còn nguyên), bỏ dòng đã khoá khỏi phép dò, hay tạo index mà không dò trước (ca trùng không lỗi).

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

func userEmailIndexExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`, userEmailCIIndex).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func newUserWithEmail(t *testing.T, db *gorm.DB, email string, active bool) uuid.UUID {
	t.Helper()
	u := model.User{Email: email, PasswordHash: "x", UserName: "qa8-" + uuid.NewString()[:8], IsActive: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("tạo user %q: %v", email, err)
	}
	if !active {
		if err := db.Model(&model.User{}).Where("id = ?", u.ID).Update("is_active", false).Error; err != nil {
			t.Fatal(err)
		}
	}
	return u.ID
}

func TestUserEmailCIUnique_CreatesIndexAndBlocksCaseVariants_Idempotent(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	for run := 1; run <= 2; run++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("lần %d RunPostMigrations: %v", run, err)
		}
	}
	if !userEmailIndexExists(t, db) {
		t.Fatalf("thiếu index %s sau RunPostMigrations", userEmailCIIndex)
	}
	base := "qa8.ci." + uuid.NewString()[:6] + "@40study.test"
	newUserWithEmail(t, db, base, true)
	err := db.Create(&model.User{Email: strings.ToUpper(base), PasswordHash: "x", UserName: "qa8-other"}).Error
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != userEmailCIIndex {
		t.Fatalf("tạo biến thể hoa/thường: err=%v, muốn 23505 trên %s", err, userEmailCIIndex)
	}
	newUserWithEmail(t, db, "other."+base, true) // email khác hẳn vẫn tạo được
}

func TestUserEmailCIUnique_DuplicatesStopLoudly_NothingChanged_ThenCleanRunSucceeds(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	// Dựng lại DB cũ: chưa có index theo lower(email), nên hai email khác hoa/thường cùng tồn tại được.
	if err := db.Exec(`DROP INDEX IF EXISTS ` + userEmailCIIndex).Error; err != nil {
		t.Fatal(err)
	}
	local := "qa8.dup." + uuid.NewString()[:6]
	keep := newUserWithEmail(t, db, local+"@40study.test", true)
	dupLocked := newUserWithEmail(t, db, strings.ToUpper(local)+"@40study.test", false) // tài khoản đã khoá cũng giữ chỗ

	err := RunPostMigrations(db)
	if err == nil {
		t.Fatal("còn email trùng không phân biệt hoa thường mà RunPostMigrations vẫn thành công")
	}
	msg := err.Error()
	for _, want := range []string{userEmailCIIndex, "1 group(s)", local + "@40study.test", keep.String(), dupLocked.String(), "No row was modified or deleted"} {
		if !strings.Contains(msg, want) {
			t.Errorf("lỗi thiếu %q: %s", want, msg)
		}
	}
	var n int64
	if err := db.Model(&model.User{}).Where("lower(email) = ?", local+"@40study.test").Count(&n).Error; err != nil || n != 2 {
		t.Errorf("còn %d dòng (err=%v), muốn 2: migration không được xoá/gộp dòng nào", n, err)
	}
	if userEmailIndexExists(t, db) {
		t.Error("index không được tạo khi còn email trùng")
	}

	// Người vận hành xử lý tay (đổi email dòng thừa) rồi khởi động lại: thành công, index được tạo.
	if err := db.Model(&model.User{}).Where("id = ?", dupLocked).Update("email", "moved."+local+"@40study.test").Error; err != nil {
		t.Fatal(err)
	}
	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("sau khi dọn trùng: %v", err)
	}
	if !userEmailIndexExists(t, db) {
		t.Fatalf("thiếu index %s sau khi dọn trùng", userEmailCIIndex)
	}
}

// QA8 review L2: index cùng phạm vi xoá mềm với idx_users_email (H3). Đỏ khi bỏ `WHERE deleted_at IS NULL` (đăng ký lại email
// của user đã xoá mềm bị 23505) hoặc bỏ bước thay index đầy đủ cũ (DB đã chạy bản trước vẫn giữ index đầy đủ).
func TestUserEmailCIUnique_PartialLikeH3_SoftDeletedDoesNotLockEmail_ReplacesFullIndex(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	if err := db.Exec(`DROP INDEX IF EXISTS ` + userEmailCIIndex + `; CREATE UNIQUE INDEX ` + userEmailCIIndex + ` ON users (lower(email))`).Error; err != nil {
		t.Fatal(err)
	}
	base := "qa8.soft." + uuid.NewString()[:6] + "@40study.test"
	gone := newUserWithEmail(t, db, base, true)
	if err := db.Delete(&model.User{}, "id = ?", gone).Error; err != nil {
		t.Fatal(err)
	}
	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}
	var def string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`, userEmailCIIndex).Scan(&def).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(def, "deleted_at IS NULL") {
		t.Fatalf("index chưa partial: %s", def)
	}
	newUserWithEmail(t, db, strings.ToUpper(base), true) // email của user đã xoá mềm dùng lại được
	err := db.Create(&model.User{Email: base, PasswordHash: "x", UserName: "qa8-" + uuid.NewString()[:8]}).Error
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != userEmailCIIndex {
		t.Fatalf("biến thể hoa/thường của user còn sống: err=%v, muốn 23505 trên %s", err, userEmailCIIndex)
	}
}
