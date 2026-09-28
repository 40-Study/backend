package database

import (
	"fmt"
	"strings"
)

// buildCheckConstraintSQL (Phase 3 duyệt khoá học/giáo viên) — bản TỔNG QUÁT của
// buildOrderStatusConstraintSQL (migrations.go): sinh khối DO $$ idempotent đảm bảo constraint
// `constraintName` trên `table` là CHECK (column IN (values...)) với ĐỦ mọi giá trị SSOT.
//
// Cùng lý do với bản orders: AutoMigrate chỉ tạo constraint còn thiếu THEO TÊN, không nới rộng
// constraint đã có trên DB cũ khi tag Go đổi (đúng trường hợp courses.status thêm 'rejected').
// Chỉ DROP+ADD khi constraint chưa tồn tại hoặc định nghĩa hiện tại THIẾU ít nhất 1 giá trị —
// không trả giá ACCESS EXCLUSIVE + validate toàn bảng mỗi lần boot.
//
// Không refactor buildOrderStatusConstraintSQL sang hàm này trong phase 3: migrations.go đang
// được lane Phase 4 cùng thêm entry song song, quy ước là chỉ THÊM dòng ở file dùng chung.
//
// table/constraintName/column/values luôn là hằng số compile-time trong code Go (SSOT ở
// internal/model), không bao giờ là input người dùng — nối chuỗi vào SQL an toàn.
func buildCheckConstraintSQL(table, constraintName, column string, values []string) string {
	quoted := make([]string, len(values))
	likeConditions := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + v + "'"
		likeConditions[i] = fmt.Sprintf("pg_get_constraintdef(oid) LIKE '%%''%s''%%'", v)
	}

	return fmt.Sprintf(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = '%s'
				  AND (%s)
			) THEN
				ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;
				ALTER TABLE %s ADD CONSTRAINT %s
					CHECK (%s IN (%s));
			END IF;
		END $$;
	`, constraintName, strings.Join(likeConditions, " AND "),
		table, constraintName,
		table, constraintName,
		column, strings.Join(quoted, ", "))
}

// buildForeignKeySQL thêm FK `constraintName` (column -> users.id) nếu chưa có. Cần riêng vì
// AutoMigrate đã tự thêm cột reviewed_by (model không khai báo quan hệ) TRƯỚC khi
// RunPostMigrations chạy, nên khối "IF NOT EXISTS column THEN ADD COLUMN ... REFERENCES" kiểu
// trong spec không bao giờ chạy tới.
func buildForeignKeySQL(table, constraintName, column string) string {
	return fmt.Sprintf(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '%s') THEN
				ALTER TABLE %s ADD CONSTRAINT %s
					FOREIGN KEY (%s) REFERENCES users(id);
			END IF;
		END $$;
	`, constraintName, table, constraintName, column)
}
