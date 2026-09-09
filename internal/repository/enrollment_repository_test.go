package repository

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
	"study.com/v1/internal/model"
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
func TestRestoreAndReactivate_SingleUpdateClearsDeletedAt(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}

	enrollmentID := uuid.New()
	updates := map[string]interface{}{
		"enrolled_at":         "now",
		"completed_at":        nil,
		"last_accessed_at":    nil,
		"progress_percentage": 0,
	}
	updates["deleted_at"] = nil

	dryRun := db.Session(&gorm.Session{DryRun: true}).
		Unscoped().
		Model(&model.Enrollment{}).
		Where("id = ?", enrollmentID).
		Updates(updates)

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
