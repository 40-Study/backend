package database

import (
	"strings"
	"testing"

	"study.com/v1/internal/model"
)

// TestBuildOrderStatusConstraintSQL (I-01, review vòng 5) — pin lại: buildOrderStatusConstraintSQL
// phải SINH đủ MỌI giá trị của model.OrderStatuses (SSOT) vào cả 2 chỗ trong SQL nó tạo ra (danh
// sách LIKE '%...%' trong điều kiện guard, VÀ danh sách 'giá trị' trong CHECK (status IN (...))),
// và phải bọc trong DO $$ ... IF NOT EXISTS (...) THEN ... END IF $$ (I-05 — tránh
// DROP+ADD CONSTRAINT vô điều kiện mỗi lần boot, xem comment tại buildOrderStatusConstraintSQL).
//
// Test này là test THUẦN GO (không cần DB Postgres thật) — chỉ kiểm HÌNH DẠNG chuỗi SQL sinh ra,
// không thực thi nó. Việc chuỗi này CHẠY ĐÚNG trên Postgres thật (cú pháp DO $$, pg_get_constraintdef
// tồn tại, v.v.) vẫn cần kiểm thủ công/staging — ghi rõ đây KHÔNG phải integration test.
//
// Tự kiểm chứng mutation (ghi lại kết quả, không giữ mutation trong working tree): sửa tạm vòng
// lặp trong buildOrderStatusConstraintSQL để BỎ QUA giá trị cuối cùng của statuses (tương đương
// lỗi review vòng 4 mutation #4 — SQL thiếu 1 giá trị) — test này FAIL đúng như kỳ vọng (thiếu
// 'expired' trong cả 2 vị trí); revert lại, chạy lại — PASS.
func TestBuildOrderStatusConstraintSQL(t *testing.T) {
	sql := buildOrderStatusConstraintSQL(model.OrderStatuses)

	if !strings.Contains(sql, "DO $$") {
		t.Errorf("expected DO $$ block (I-05, guard chống chạy vô điều kiện mỗi lần boot), got SQL: %s", sql)
	}
	if !strings.Contains(sql, "IF NOT EXISTS") {
		t.Errorf("expected IF NOT EXISTS guard, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "chk_orders_status") {
		t.Errorf("expected constraint name chk_orders_status referenced, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "DROP CONSTRAINT IF EXISTS chk_orders_status") ||
		!strings.Contains(sql, "ADD CONSTRAINT chk_orders_status") {
		t.Errorf("expected DROP+ADD CONSTRAINT pair inside the guard, got SQL: %s", sql)
	}

	for _, status := range model.OrderStatuses {
		if !strings.Contains(sql, "LIKE '%"+status+"%'") {
			t.Errorf("expected guard LIKE condition for status %q (from model.OrderStatuses/SSOT), got SQL: %s", status, sql)
		}
		if !strings.Contains(sql, "'"+status+"'") {
			t.Errorf("expected status %q quoted in CHECK (status IN (...)) value list, got SQL: %s", status, sql)
		}
	}
}

// TestBuildOrderStatusConstraintSQL_EmptyInput — biên: statuses rỗng không panic, sinh SQL với
// danh sách rỗng (sẽ lỗi ở Postgres thật — CHECK (status IN ()) không hợp lệ cú pháp — nhưng
// không phải mối lo thực tế vì model.OrderStatuses không bao giờ rỗng; test chỉ đảm bảo hàm
// không panic trên input biên, không giả lập DB thật để kiểm cú pháp).
func TestBuildOrderStatusConstraintSQL_EmptyInput(t *testing.T) {
	sql := buildOrderStatusConstraintSQL(nil)
	if !strings.Contains(sql, "DO $$") {
		t.Errorf("expected function not to panic and still emit a DO $$ block for empty input, got: %s", sql)
	}
}
