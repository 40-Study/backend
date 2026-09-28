package database

import (
	"fmt"
	"strings"
)

// buildStatusCheckConstraintSQL sinh khối DO $$ idempotent đặt CHECK (column IN (...)) cho 1 cột
// trạng thái, từ 1 slice SSOT trong internal/model (Phase 4: model.PayoutStatuses).
//
// Tổng quát hoá từ buildOrderStatusConstraintSQL (migrations.go) thành file riêng thay vì sửa hàm
// cũ — migrations.go đang được nhiều lane sửa song song, chỉ được THÊM entry.
//
// Khác hàm cũ ở 1 điểm quan trọng: hàm cũ chỉ phát hiện constraint THIẾU giá trị (LIKE từng giá
// trị). Phase 4 thì phải BỎ giá trị ('processing', 'failed'), nên còn kiểm thêm số giá trị: mỗi
// giá trị xuất hiện đúng 1 lần trong pg_get_constraintdef dưới dạng literal có 2 dấu nháy đơn,
// nên số dấu nháy phải bằng 2*len(values). Constraint cũ thừa giá trị -> số nháy lệch -> DROP+ADD.
// Chỉ DROP+ADD khi lệch (ALTER TABLE ... ADD CONSTRAINT khoá ACCESS EXCLUSIVE + validate cả bảng,
// không nên chạy mỗi lần boot).
//
// table/constraint/column/values đều là hằng compile-time trong code Go, không phải input người
// dùng, nên nối chuỗi thẳng vào SQL là an toàn.
func buildStatusCheckConstraintSQL(table, constraint, column string, values []string) string {
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
				  AND %s
			) THEN
				ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;
				ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s IN (%s));
			END IF;
		END $$;
	`, constraint, strings.Join(conditions, "\n\t\t\t\t  AND "),
		table, constraint, table, constraint, column, strings.Join(quoted, ", "))
}
