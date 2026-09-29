package database

import "study.com/v1/internal/model"

// contestPostMigrations — MVP "Cuộc thi" (contract §1.7). Tách file riêng để migrations.go
// không phình thêm; RunPostMigrations nối danh sách này vào CUỐI.
//
// Thứ tự BẮT BUỘC trên DB cũ: constraint chk_contests_status cũ chỉ cho phép
// DRAFT/UPCOMING/ACTIVE/ENDED/CANCELLED, nên UPDATE sang 'PUBLISHED' sẽ vi phạm nó. Vì vậy phải
// (1) gỡ constraint cũ khi nó CHƯA biết 'PUBLISHED' → (2) chuyển dữ liệu cũ → (3) tạo constraint
// mới sinh từ model.ContestStatuses. Contract ghi (2) trước (3) nhưng không nhắc (1); thiếu (1)
// thì bước (2) lỗi 23514 và API không khởi động được trên DB đang có cuộc thi cũ.
func contestPostMigrations() []struct {
	name string
	sql  string
} {
	return []struct {
		name string
		sql  string
	}{
		{
			name: "drop legacy chk_contests_status without PUBLISHED (contest MVP)",
			sql: `
				DO $$
				BEGIN
					IF EXISTS (
						SELECT 1 FROM pg_constraint
						WHERE conname = 'chk_contests_status'
						  AND conrelid = 'contests'::regclass
						  AND pg_get_constraintdef(oid) NOT LIKE '%''PUBLISHED''%'
					) THEN
						ALTER TABLE contests DROP CONSTRAINT chk_contests_status;
					END IF;
				END $$;
			`,
		},
		{
			name: "migrate legacy contest statuses to PUBLISHED (contest MVP)",
			sql:  `UPDATE contests SET status = 'PUBLISHED' WHERE status IN ('UPCOMING','ACTIVE','ENDED');`,
		},
		{
			name: "sync chk_contests_status to model.ContestStatuses (contest MVP)",
			sql:  buildCheckConstraintSQL("contests", "chk_contests_status", "status", model.ContestStatuses),
		},
		{
			name: "fk contests.reviewed_by -> users (contest MVP)",
			sql:  buildForeignKeySQL("contests", "fk_contests_reviewed_by", "reviewed_by"),
		},
		{
			name: "fk contests.finalized_by -> users (contest MVP)",
			sql:  buildForeignKeySQL("contests", "fk_contests_finalized_by", "finalized_by"),
		},
		{
			// quizzes.created_by do lane B2 thêm vào model.Quiz (AutoMigrate tạo cột). Khối này là
			// lớp phòng thủ idempotent: bảo đảm cột + index tồn tại TRƯỚC khi gắn FK dù thứ tự
			// merge/AutoMigrate thay đổi (cùng khuôn users.locked_* ở migrations.go).
			name: "ensure quizzes.created_by column + index (contest MVP)",
			sql: `
				DO $$
				BEGIN
					IF NOT EXISTS (SELECT 1 FROM information_schema.columns
					               WHERE table_schema = current_schema() AND table_name = 'quizzes'
					                 AND column_name = 'created_by') THEN
						ALTER TABLE quizzes ADD COLUMN created_by UUID;
					END IF;
				END $$;
				CREATE INDEX IF NOT EXISTS idx_quizzes_created_by ON quizzes (created_by);
			`,
		},
		{
			name: "fk quizzes.created_by -> users (contest MVP)",
			sql:  buildForeignKeySQL("quizzes", "fk_quizzes_created_by", "created_by"),
		},
	}
}
