package database

// Test Postgres THẬT (schema tạm) cho unique index lower(users.email) (user_email_ci_unique.go), gọi qua RunPostMigrations như
// lúc API khởi động (QA8 PG-01, C18). Đỏ khi: bỏ lời gọi khỏi RunPostMigrations (ca index), đổi sang tự xoá/đổi email dòng trùng
// (ca dừng, kiểm email còn nguyên), bỏ dòng đã khoá khỏi phép dò, hay tạo index mà không dò trước (ca trùng không lỗi).

import (
	"errors"
	"fmt"
	"strings"
	"sync"
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

// Hồi quy CI QA8 (PR #157): bước "drop full uq_users_email_ci" từng dò qua view pg_indexes, view gọi pg_get_indexdef() trên
// index của MỌI schema. Gói test khác đang tạo/DROP schema tạm song song thì index biến mất giữa chừng và lệnh chết với
// "could not open relation with OID n" (SQLSTATE XX000), làm ~30 test đỏ trên CI. Test này tạo/xoá index ở schema khác liên tục
// trong lúc chạy bước xoá nhiều lần; đỏ nếu phép dò lại đụng tới index ngoài current_schema().
func TestDropFullUserEmailCIIndex_ConcurrentSchemaChurn_NeverTouchesOtherSchemas(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	admin := pgtest.Open(t)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	// Schema nhiễu giống schema tạm thật: nhiều bảng, mỗi bảng một index biểu thức, rồi DROP CASCADE một phát.
	churnSQL := `DO $$ BEGIN
		EXECUTE 'CREATE SCHEMA ' || quote_ident('%[1]s');
		FOR i IN 1..60 LOOP
			EXECUTE format('CREATE TABLE %%I.t%%s (a text)', '%[1]s', i);
			EXECUTE format('CREATE INDEX ON %%I.t%%s (lower(a))', '%[1]s', i);
		END LOOP;
		-- Cùng tên index với schema thật: phép dò theo indexname (không theo schema) sẽ khớp cả bản này.
		EXECUTE format('CREATE TABLE %%I.users (email text)', '%[1]s');
		EXECUTE format('CREATE UNIQUE INDEX ` + userEmailCIIndex + ` ON %%I.users (lower(email))', '%[1]s');
	END $$`
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				name := "churn_" + strings.ReplaceAll(uuid.NewString()[:12], "-", "")
				// Lỗi của luồng nhiễu không phải đối tượng kiểm: chỉ cần nó liên tục tạo/xoá index ở schema khác.
				_ = admin.Exec(fmt.Sprintf(churnSQL, name)).Error
				_ = admin.Exec(`DROP SCHEMA IF EXISTS ` + name + ` CASCADE`).Error
			}
		}()
	}
	defer func() {
		close(stop)
		wg.Wait()
	}()

	for i := 0; i < 400; i++ {
		if err := db.Exec(dropFullUserEmailCIIndexSQL).Error; err != nil {
			t.Fatalf("lần %d: bước xoá index đầy đủ chạm index schema khác: %v", i, err)
		}
	}
}

// Idempotent trên DB mới: chưa có index thì bước xoá không làm gì, không lỗi.
func TestDropFullUserEmailCIIndex_NoIndexYet_NoOp(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	if err := db.Exec(`DROP INDEX IF EXISTS ` + userEmailCIIndex).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := db.Exec(dropFullUserEmailCIIndexSQL).Error; err != nil {
			t.Fatalf("lần %d: %v", i, err)
		}
	}
	if userEmailIndexExists(t, db) {
		t.Fatalf("index %s không được tự xuất hiện", userEmailCIIndex)
	}
}
