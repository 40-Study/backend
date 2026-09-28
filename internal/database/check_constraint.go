package database

import (
	"fmt"
	"strings"
)

// buildCheckConstraintSQL (Phase 3 duyệt khoá học/giáo viên) — bản TỔNG QUÁT của
// buildOrderStatusConstraintSQL (migrations.go): sinh khối DO $$ idempotent đảm bảo constraint
// `constraintName` trên `table` là CHECK (column IN (values...)) với ĐỦ mọi giá trị SSOT.
//
// Cùng lý do với bản orders: AutoMigrate chỉ tạo constraint còn thiếu THEO TÊN, không sửa
// constraint đã có trên DB cũ khi tag Go đổi (courses.status thêm 'rejected' ở Phase 3,
// instructor_payouts.status bỏ 'processing'/'failed' ở Phase 4).
//
// "Đã đúng" = constraint của CHÍNH bảng này (conrelid, không nhầm với bảng cùng tên constraint ở
// schema khác) chứa ĐỦ mọi giá trị (LIKE từng giá trị có bọc nháy) VÀ không THỪA giá trị nào: mỗi
// giá trị xuất hiện đúng 1 lần trong pg_get_constraintdef dưới dạng literal có 2 dấu nháy đơn,
// nên số dấu nháy phải bằng 2*len(values) — constraint cũ thừa giá trị thì số nháy lệch. Chỉ
// DROP+ADD khi lệch, không trả giá ACCESS EXCLUSIVE + validate toàn bảng mỗi lần boot.
//
// Gộp từ 2 helper song song của Phase 3 (#73, chỉ kiểm thiếu) và Phase 4 (#74, kiểm cả thừa).
//
// table/constraintName/column/values luôn là hằng số compile-time trong code Go (SSOT ở
// internal/model), không bao giờ là input người dùng — nối chuỗi vào SQL an toàn.
func buildCheckConstraintSQL(table, constraintName, column string, values []string) string {
	quoted := make([]string, len(values))
	conditions := make([]string, 0, len(values)+1)
	for i, v := range values {
		quoted[i] = "'" + v + "'"
		conditions = append(conditions, fmt.Sprintf("pg_get_constraintdef(oid) LIKE '%%''%s''%%'", v))
	}
	conditions = append(conditions, fmt.Sprintf(
		"length(pg_get_constraintdef(oid)) - length(replace(pg_get_constraintdef(oid), '''', '')) = %d",
		2*len(values)))

	return fmt.Sprintf(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = '%s'
				  AND conrelid = '%s'::regclass
				  AND %s
			) THEN
				ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;
				ALTER TABLE %s ADD CONSTRAINT %s
					CHECK (%s IN (%s));
			END IF;
		END $$;
	`, constraintName, table, strings.Join(conditions, "\n\t\t\t\t  AND "),
		table, constraintName,
		table, constraintName,
		column, strings.Join(quoted, ", "))
}

// buildNormalizeStatusSQL đưa mọi giá trị ngoài `values` (kể cả NULL) của `column` về `fallback`,
// để buildCheckConstraintSQL chạy sau đó không lỗi trên DB cũ có dữ liệu rác (review PR #81,
// MINOR-8). Cùng điều kiện an toàn như trên: tham số luôn là hằng số trong code Go.
func buildNormalizeStatusSQL(table, column, fallback string, values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + v + "'"
	}
	return fmt.Sprintf(`UPDATE %s SET %s = '%s' WHERE %s IS NULL OR %s NOT IN (%s)`,
		table, column, fallback, column, column, strings.Join(quoted, ", "))
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
