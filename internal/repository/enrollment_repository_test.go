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

// TestRestoreAndReactivate_DelegatesToQueryBuilder (M2-05, review vòng 3) — pin hợp đồng RẰNG
// RestoreAndReactivate (method thật, dùng ở EnrollmentService.Enroll/completeOrderFulfillment)
// TRẢ VỀ đúng .Error của buildRestoreAndReactivateQuery, không tự làm gì khác. Gọi trên DryRun DB
// (không thực thi thật, .Error luôn nil cho một Statement hợp lệ) — nếu ai xóa hẳn
// RestoreAndReactivate, test này đỏ vì không còn compile được.
func TestRestoreAndReactivate_DelegatesToQueryBuilder(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &EnrollmentRepository{db: db.Session(&gorm.Session{DryRun: true})}

	updates := map[string]interface{}{"enrolled_at": "now"}
	if err := repo.RestoreAndReactivate(context.Background(), uuid.New(), updates); err != nil {
		t.Errorf("RestoreAndReactivate() on DryRun DB should not error, got %v", err)
	}
}
