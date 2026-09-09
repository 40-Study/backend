package database

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// buildOrderStatusConstraintSQL (I-01 + I-05, review vòng 5) — SINH câu SQL của constraint
// chk_orders_status TỪ statuses (gọi với model.OrderStatuses — SSOT, internal/model/
// order_status.go) thay vì tự chép lại một chuỗi literal độc lập như TRƯỚC vòng 5. Bằng chứng
// review vòng 4 (mutation #4): bản literal cũ xóa 'expired' đi mà không có test nào bắt được —
// SINH ĐỘNG từ slice khiến lớp lỗi đó KHÔNG THỂ xảy ra nữa (sửa migrations.go một mình không đủ
// để làm SQL lệch khỏi statuses — muốn lệch phải sửa CHÍNH statuses, mà sửa statuses thì
// TestOrderStatusTagMatchesSSOT bên internal/model đỏ ngay vì tag không đổi theo).
//
// I-05: TRƯỚC ĐÂY DROP CONSTRAINT IF EXISTS + ADD CONSTRAINT chạy VÔ ĐIỀU KIỆN mỗi lần
// RunPostMigrations được gọi (tức mỗi lần API khởi động — internal/app/resources.go) — kết quả
// idempotent (chạy lại nhiều lần vẫn ra constraint giống hệt) NHƯNG chi phí thì KHÔNG: ALTER
// TABLE ... ADD CONSTRAINT CHECK khoá ACCESS EXCLUSIVE và VALIDATE TOÀN BỘ bảng orders, ngay cả
// khi constraint đã đúng sẵn từ lần chạy trước. Bọc DO $$ ... IF NOT EXISTS (...) THEN ... END
// IF $$ — cùng khuôn với chk_coin_wallet_balance_nonneg (thống kê bên dưới) — chỉ khác:
// chk_coin_wallet_balance_nonneg chỉ kiểm TỒN TẠI THEO TÊN (constraint đó không bao giờ đổi nội
// dung sau khi tạo), còn chk_orders_status có thể tồn tại nhưng SAI NỘI DUNG (đúng lỗ hổng
// B3-01 gốc — dữ liệu cũ có constraint tên đúng nhưng thiếu 'expired') nên phải kiểm THÊM nội
// dung định nghĩa hiện tại (pg_get_constraintdef) có chứa ĐỦ MỌI giá trị trong statuses không —
// LIKE '%<status>%' cho từng giá trị (Postgres render CHECK IN (...) thành dạng
// "= ANY (ARRAY[...])" khi đọc lại qua pg_get_constraintdef, nên so khớp CHÍNH XÁC toàn chuỗi là
// giòn/dễ vỡ theo version Postgres; kiểm SUBSTRING từng giá trị là đủ để phát hiện "thiếu 1 giá
// trị" mà không phụ thuộc cách Postgres canonical-hoá cú pháp). Chỉ khi THIẾU (constraint chưa
// tồn tại HOẶC thiếu ít nhất 1 giá trị) mới trả giá DROP+ADD — đúng 1 lần trên mỗi DB, không
// phải mỗi lần boot.
func buildOrderStatusConstraintSQL(statuses []string) string {
	quoted := make([]string, len(statuses))
	likeConditions := make([]string, len(statuses))
	for i, s := range statuses {
		// statuses luôn đến từ model.OrderStatuses — hằng số compile-time trong code Go, KHÔNG
		// phải input người dùng, nên nối chuỗi trực tiếp vào SQL literal an toàn (không có input
		// nào từ bên ngoài chạm tới hàm này).
		quoted[i] = "'" + s + "'"
		likeConditions[i] = fmt.Sprintf("pg_get_constraintdef(oid) LIKE '%%%s%%'", s)
	}
	valueList := strings.Join(quoted, ", ")
	allValuesPresent := strings.Join(likeConditions, " AND ")

	return fmt.Sprintf(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'chk_orders_status'
				  AND (%s)
			) THEN
				ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_status;
				ALTER TABLE orders ADD CONSTRAINT chk_orders_status
					CHECK (status IN (%s));
			END IF;
		END $$;
	`, allValuesPresent, valueList)
}

// RunPostMigrations chạy các câu SQL idempotent SAU khi AutoMigrate xong, để sửa
// những thứ AutoMigrate không tự sửa được: đổi tên/xoá index sai, chuyển unique
// index thường thành PARTIAL index (WHERE deleted_at IS NULL) trên bảng có soft
// delete, và thêm CHECK constraint cho bảng đã tồn tại dữ liệu.
//
// Lý do cần file này: GORM AutoMigrate chỉ TẠO MỚI cột/index/constraint còn thiếu
// (kiểm tra bằng Migrator().HasIndex/HasConstraint theo TÊN); nếu 1 index đã tồn
// tại dưới tên đó nhưng SAI cấu trúc cột (vd: unique 1 cột thay vì unique 2 cột),
// AutoMigrate coi như "đã có" và bỏ qua, để lại cấu trúc sai vĩnh viễn. Dự án này
// chưa dùng công cụ migration có version (golang-migrate/goose/atlas — xem M1
// trong báo cáo audit) nên RunPostMigrations đóng vai trò migration thủ công tối
// thiểu, thay thế tạm thời cho tới khi có công cụ migration thật.
//
// Mọi câu lệnh đều idempotent (DROP INDEX IF EXISTS, CREATE ... IF NOT EXISTS,
// khối DO $$ kiểm tra pg_constraint trước khi ALTER TABLE ADD CONSTRAINT) nên
// chạy lại nhiều lần trên cùng 1 DB không lỗi, không nhân đôi.
func RunPostMigrations(db *gorm.DB) error {
	statements := []struct {
		name string
		sql  string
	}{
		{
			// C1/H3: reviews trước đây unique trên course_id ĐƠN LẺ (bug: chỉ 1
			// người trên toàn hệ thống review được mỗi khoá học). Đổi thành unique
			// (user_id, course_id) và PARTIAL (WHERE deleted_at IS NULL) vì Review
			// có soft delete — nếu không partial, xoá review rồi review lại sẽ bị
			// lỗi 23505 duplicate key (H3).
			name: "fix idx_user_course_review (C1 + H3)",
			sql: `
				DROP INDEX IF EXISTS idx_user_course_review;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_user_course_review
					ON reviews (user_id, course_id) WHERE deleted_at IS NULL;
			`,
		},
		{
			// H2: user_oauth_providers trước đây unique trên provider_user_id ĐƠN
			// LẺ — 1 provider_user_id từ Google chặn luôn cùng id đó từ GitHub.
			// user_oauth_providers không có soft delete nên không cần partial.
			name: "fix idx_oauth_provider_user (H2)",
			sql: `
				DROP INDEX IF EXISTS idx_oauth_provider_user;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_oauth_provider_user
					ON user_oauth_providers (provider, provider_user_id);
			`,
		},
		{
			// H1: idx_student_class từng bị khai trùng tên ở CẢ student_classes lẫn
			// final_grades — PostgreSQL yêu cầu tên index duy nhất theo schema, nên
			// AutoMigrate tạo cái thứ 2 sẽ lỗi và 1 trong 2 bảng mất unique
			// constraint tuỳ thứ tự migrate. Model đã đổi sang 2 tên riêng
			// (idx_student_class_unique / idx_final_grade_student_class, AutoMigrate
			// tự tạo vì là tên MỚI) — ở đây chỉ cần dọn index tên cũ còn sót lại.
			name: "drop legacy idx_student_class (H1)",
			sql:  `DROP INDEX IF EXISTS idx_student_class;`,
		},
		{
			// H3: các unique index còn lại trên bảng CÓ soft delete (BaseModel) —
			// chuyển sang partial để huỷ/xoá mềm rồi tạo lại không bị 23505.
			// enrollments: huỷ ghi danh rồi ghi danh lại từng luôn lỗi 500 (C5).
			name: "partial unique idx_user_course on enrollments (H3, fixes C5)",
			sql: `
				DROP INDEX IF EXISTS idx_user_course;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_user_course
					ON enrollments (user_id, course_id) WHERE deleted_at IS NULL;
			`,
		},
		{
			name: "partial unique idx_user_lesson on lesson_progress (H3)",
			sql: `
				DROP INDEX IF EXISTS idx_user_lesson;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_user_lesson
					ON lesson_progress (user_id, lesson_id) WHERE deleted_at IS NULL;
			`,
		},
		{
			// users.email: xoá mềm 1 user không được "khoá" email đó vĩnh viễn.
			name: "partial unique idx_users_email on users (H3)",
			sql: `
				DROP INDEX IF EXISTS idx_users_email;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email
					ON users (email) WHERE deleted_at IS NULL;
			`,
		},
		{
			name: "partial unique idx_courses_slug on courses (H3)",
			sql: `
				DROP INDEX IF EXISTS idx_courses_slug;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_courses_slug
					ON courses (slug) WHERE deleted_at IS NULL;
			`,
		},
		{
			name: "partial unique idx_groups_slug on groups (H3)",
			sql: `
				DROP INDEX IF EXISTS idx_groups_slug;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_groups_slug
					ON groups (slug) WHERE deleted_at IS NULL;
			`,
		},
		{
			name: "partial unique idx_contests_slug on contests (H3)",
			sql: `
				DROP INDEX IF EXISTS idx_contests_slug;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_contests_slug
					ON contests (slug) WHERE deleted_at IS NULL;
			`,
		},
		{
			// C4: chặn số dư ví xu âm ở tầng DB — guard duy nhất trước đây chỉ nằm ở
			// application code (SubtractBalance WHERE balance >= ?), có thể bị vô
			// hiệu nếu code gọi sai tham số. Bọc DO $$ kiểm tra pg_constraint trước
			// vì ALTER TABLE ... ADD CONSTRAINT không có cú pháp "IF NOT EXISTS".
			name: "check constraint chk_coin_wallet_balance_nonneg (C4)",
			sql: `
				DO $$
				BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM pg_constraint WHERE conname = 'chk_coin_wallet_balance_nonneg'
					) THEN
						ALTER TABLE user_coin_wallets
							ADD CONSTRAINT chk_coin_wallet_balance_nonneg CHECK (balance >= 0);
					END IF;
				END $$;
			`,
		},
		{
			// B3-01 (review vòng 4) + I-01/I-05 (review vòng 5): widen CHECK constraint của
			// orders.status để khớp model.OrderStatuses (SSOT, internal/model/order_status.go).
			// AutoMigrate CHỈ tạo mới constraint còn thiếu theo TÊN (Migrator().HasConstraint),
			// không nới rộng constraint đã tồn tại trên DB cũ dù tag Go đã đổi. Tên constraint
			// chk_orders_status khớp quy ước đặt tên mặc định của GORM cho check tag không có
			// tên tường minh (chk_<table>_<column>). SQL được SINH TỪ model.OrderStatuses (xem
			// buildOrderStatusConstraintSQL ở trên) — KHÔNG còn chép tay danh sách 7 giá trị ở
			// đây nữa (I-01), và chỉ DROP+ADD khi định nghĩa hiện tại THIẾU giá trị nào đó thay
			// vì chạy vô điều kiện mỗi lần boot (I-05, tránh ACCESS EXCLUSIVE + validate toàn
			// bảng orders lặp lại không cần thiết).
			name: "widen chk_orders_status to match model.OrderStatuses (B3-01/I-01/I-05)",
			sql:  buildOrderStatusConstraintSQL(model.OrderStatuses),
		},
	}

	for _, stmt := range statements {
		if err := db.Exec(stmt.sql).Error; err != nil {
			return fmt.Errorf("post-migration %q failed: %w", stmt.name, err)
		}
	}

	log.Printf("Post-migrations applied: %d statement group(s)\n", len(statements))
	return nil
}
