package database

// class_created_by_backfill.go — gán chủ cho lớp tạo trước S4 (classes.created_by NULL). Không backfill
// thì lớp độc lập cũ (course_id NULL) chỉ còn admin gán/gỡ giảng viên (review S4, M-1). Chủ = giảng viên
// role primary được gán SỚM NHẤT của lớp (assigned_at, hoà thì id nhỏ hơn cho kết quả xác định).
//
// Idempotent: chỉ chạm dòng created_by IS NULL, nên chạy lại ở mỗi lần API khởi động không đổi gì và
// không ghi đè chủ đã có. Lớp không có giảng viên primary giữ NULL (chỉ chủ khoá và admin quản lý).

import (
	"fmt"

	"gorm.io/gorm"
)

const classCreatedByBackfillSQL = `
	UPDATE classes c
	SET created_by = t.teacher_id
	FROM (
		SELECT DISTINCT ON (class_id) class_id, teacher_id
		FROM teacher_classes
		WHERE role = 'primary'
		ORDER BY class_id, assigned_at ASC, id ASC
	) t
	WHERE c.created_by IS NULL AND c.id = t.class_id;
`

// runClassCreatedByBackfill gọi từ RunPostMigrations sau AutoMigrate (đã thêm cột created_by).
func runClassCreatedByBackfill(db *gorm.DB) error {
	if err := db.Exec(classCreatedByBackfillSQL).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "backfill classes.created_by from earliest primary teacher", err)
	}
	return nil
}