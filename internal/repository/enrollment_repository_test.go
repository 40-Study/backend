package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

// TestRestoreAndReactivate_SingleUpdateClearsDeletedAt (H-02, review vòng 1) — pin lại đúng
// hình dạng câu SQL của RestoreAndReactivate: PHẢI là một câu UPDATE duy nhất có "deleted_at"
// trong SET, cùng với các field reset tiến trình học được truyền vào qua map.
//
// Bug gốc: EnrollmentService.Enroll gọi Restore() (UPDATE deleted_at=NULL) rồi gọi tiếp
// enrollmentRepo.Update(existing) = db.Save(existing) trên struct đã load TRƯỚC khi Restore
// chạy — struct đó vẫn giữ DeletedAt cũ trong bộ nhớ nên Save() ghi đè lại đúng giá trị cũ,
// vô hiệu hóa Restore() ngay lập tức (2 câu UPDATE tách rời, câu sau đè câu trước). Sửa bằng
// cách gộp vào MỘT Updates(map) duy nhất — test này pin lại: chỉ có MỘT câu SQL, và câu đó
// chứa cả "deleted_at" lẫn field truyền vào.
//
// Dùng DummyDialector (đã có sẵn trong module gorm, không thêm dependency) ở chế độ DryRun,
// giống pattern coupon_repository_test.go — không cần Postgres thật.
//
// M2-05 (review vòng 3): TRƯỚC ĐÂY test này hand-roll lại câu query bằng tay
// (db.Model(...).Where(...).Updates(...)) thay vì gọi RestoreAndReactivate thật — xóa hẳn
// RestoreAndReactivate vẫn không làm test này đỏ ("green that proves nothing"). Sửa bằng cách
// gọi buildRestoreAndReactivateQuery — hàm PRODUCTION thật mà RestoreAndReactivate ủy quyền tới
// (xem enrollment_repository.go) — trên DryRun DB rồi đọc Statement.SQL từ kết quả trả về.
func TestRestoreAndReactivate_SingleUpdateClearsDeletedAt(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &EnrollmentRepository{db: db.Session(&gorm.Session{DryRun: true})}

	enrollmentID := uuid.New()
	updates := map[string]interface{}{
		"enrolled_at":         "now",
		"completed_at":        nil,
		"last_accessed_at":    nil,
		"progress_percentage": 0,
	}

	dryRun := repo.buildRestoreAndReactivateQuery(context.Background(), enrollmentID, updates)

	sql := dryRun.Statement.SQL.String()

	if !strings.Contains(sql, "UPDATE") {
		t.Fatalf("expected UPDATE statement, got: %s", sql)
	}
	if !strings.Contains(sql, "deleted_at") {
		t.Errorf("expected deleted_at to be reset in SET clause (H-02 fix), got SQL: %s", sql)
	}
	if !strings.Contains(sql, "progress_percentage") {
		t.Errorf("expected progress_percentage to be reset in same UPDATE, got SQL: %s", sql)
	}
	// Unscoped() phải loại bỏ soft-delete scope khỏi WHERE, nếu không UPDATE sẽ không khớp
	// được đúng bản ghi đang bị soft-delete (deleted_at IS NULL sẽ loại nó ra khỏi WHERE).
	if strings.Contains(sql, `"enrollments"."deleted_at" IS NULL`) {
		t.Errorf("expected Unscoped() to remove default soft-delete WHERE clause, got SQL: %s", sql)
	}
}

// TestRestoreAndReactivate_DelegatesToQueryBuilder (M2-05 vòng 3, đã sửa lại ở H3-02 vòng 4) —
// vòng 3 CHỈ assert `err == nil` trên DryRun DB, mà DryRun không thực thi gì cả nên `err == nil`
// đúng với BẤT KỲ implementation nào — kể cả `func RestoreAndReactivate(...) error { return nil }`
// rỗng hoàn toàn. Reviewer vòng 4 (H3-02) đã CHỨNG MINH bằng mutation test thật: gutting thân hàm
// thành `return nil` vẫn để cả 2 test trong file này xanh. Tên hàm "DelegatesToQueryBuilder" hứa
// hẹn kiểm tra HÀNH VI ủy quyền, nhưng bản cũ chỉ là compile-time reference check.
//
// Sửa: gọi ĐÚNG RestoreAndReactivate thật (không phải buildRestoreAndReactivateQuery như test ở
// trên) và bắt câu SQL nó SINH RA bằng GORM callback — RestoreAndReactivate chỉ trả về `.Error`,
// không trả `*gorm.DB`, nên không có cách nào đọc Statement.SQL từ giá trị trả về; đăng ký một
// callback chạy SAU "gorm:update" (callback mặc định GORM dùng để build+chạy UPDATE, kể cả ở chế
// độ DryRun — DryRun chỉ bỏ qua bước Exec thật, KHÔNG bỏ qua việc build Statement.SQL hay các
// callback đăng ký sau đó) để "chụp" lại Statement.SQL ngay khi RestoreAndReactivate thực thi.
// Callback không bao giờ fire nếu thân hàm bị gut thành `return nil` (không còn UPDATE nào chạy
// qua callback chain) — capturedSQL vẫn rỗng, cả 2 assertion strings.Contains bên dưới đỏ ngay.
//
// Đã tự kiểm chứng: sửa tạm RestoreAndReactivate thành `return nil`, chạy lại — test này FAIL
// đúng như mong đợi (capturedSQL == ""); revert lại nguyên trạng, chạy lại — PASS. Không giữ
// lại bản mutation trong working tree.
func TestRestoreAndReactivate_DelegatesToQueryBuilder(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	dryRunDB := db.Session(&gorm.Session{DryRun: true})

	var capturedSQL string
	if err := dryRunDB.Callback().Update().After("gorm:update").
		Register("test:capture_sql", func(tx *gorm.DB) {
			capturedSQL = tx.Statement.SQL.String()
		}); err != nil {
		t.Fatalf("failed to register capture callback: %v", err)
	}

	repo := &EnrollmentRepository{db: dryRunDB}
	updates := map[string]interface{}{"enrolled_at": "now"}
	enrollmentID := uuid.New()

	if err := repo.RestoreAndReactivate(context.Background(), enrollmentID, updates); err != nil {
		t.Fatalf("RestoreAndReactivate() on DryRun DB unexpected error: %v", err)
	}

	// DummyDialector quote identifier bằng backtick (kiểu MySQL: `enrollments`), không phải
	// dấu nháy kép kiểu Postgres thật — không giả định ký tự quote, chỉ kiểm tên bảng xuất hiện.
	if !strings.Contains(capturedSQL, "UPDATE") || !strings.Contains(capturedSQL, "enrollments") {
		t.Fatalf("expected RestoreAndReactivate to actually run an UPDATE ... enrollments ... statement, captured SQL: %q", capturedSQL)
	}
	if !strings.Contains(capturedSQL, "deleted_at") {
		t.Errorf("expected deleted_at reset in SET clause (H-02), captured SQL: %q", capturedSQL)
	}
}
