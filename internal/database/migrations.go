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
			// Smoke test 11/09/2026 trên DB dev: vouchers.code unique THƯỜNG — admin xóa (mềm) một
			// voucher rồi tạo lại cùng mã bị "duplicated key not allowed" vĩnh viễn. Cùng lớp H3.
			name: "partial unique idx_vouchers_code on vouchers (H3)",
			sql: `
				DROP INDEX IF EXISTS idx_vouchers_code;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_vouchers_code
					ON vouchers (code) WHERE deleted_at IS NULL;
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
		{
			// Phase 1 quản lý người dùng (2026-09-28): cột audit khoá/mở tài khoản trên
			// users. AutoMigrate (postgres.go) đã tự thêm 3 cột này từ model.User (ADD
			// COLUMN cho model đã tồn tại) — khối DO $$ ở đây là lớp phòng thủ tường minh,
			// idempotent, cùng khuôn với các entry khác trong file này, đề phòng thứ tự
			// AutoMigrate thay đổi trong tương lai.
			name: "add locked_reason/locked_at/locked_by columns to users (phase-1 user mgmt)",
			sql: `
				DO $$
				BEGIN
					IF NOT EXISTS (SELECT 1 FROM information_schema.columns
					               WHERE table_name='users' AND column_name='locked_reason') THEN
						ALTER TABLE users ADD COLUMN locked_reason TEXT;
					END IF;
					IF NOT EXISTS (SELECT 1 FROM information_schema.columns
					               WHERE table_name='users' AND column_name='locked_at') THEN
						ALTER TABLE users ADD COLUMN locked_at TIMESTAMP;
					END IF;
					IF NOT EXISTS (SELECT 1 FROM information_schema.columns
					               WHERE table_name='users' AND column_name='locked_by') THEN
						ALTER TABLE users ADD COLUMN locked_by UUID REFERENCES users(id);
					END IF;
				END $$;
			`,
		},
		{
			// Phase 3 duyệt khoá học (2026-09-28): nới chk_courses_status thêm 'rejected' — SQL
			// SINH từ model.CourseStatuses (SSOT, course_status.go). AutoMigrate KHÔNG nới rộng
			// constraint đã tồn tại theo tên trên DB cũ.
			name: "widen chk_courses_status to match model.CourseStatuses (phase-3 approval)",
			sql:  buildCheckConstraintSQL("courses", "chk_courses_status", "status", model.CourseStatuses),
		},
		{
			// Phase 3: FK cho courses.reviewed_by (cột do AutoMigrate thêm, không kèm FK).
			name: "fk courses.reviewed_by -> users (phase-3 approval)",
			sql:  buildForeignKeySQL("courses", "fk_courses_reviewed_by", "reviewed_by"),
		},
		{
			// Phase 3 duyệt giáo viên: CHECK approval_status sinh từ model.TeacherApprovalStatuses.
			name: "chk_teacher_profiles_approval_status (phase-3 approval)",
			sql: buildCheckConstraintSQL("teacher_profiles", "chk_teacher_profiles_approval_status",
				"approval_status", model.TeacherApprovalStatuses),
		},
		{
			name: "fk teacher_profiles.reviewed_by -> users (phase-3 approval)",
			sql:  buildForeignKeySQL("teacher_profiles", "fk_teacher_profiles_reviewed_by", "reviewed_by"),
		},
		{
			// Phase 3: AutoMigrate thêm approval_status DEFAULT 'pending' cho MỌI hồ sơ cũ — kể
			// cả hồ sơ của giáo viên ĐANG dạy thật, khiến họ lọt vào hàng chờ duyệt. Hồ sơ nào mà
			// chủ đã giữ role TEACHER active thì coi như đã duyệt. Điều kiện theo DỮ LIỆU (không
			// theo "cột vừa được thêm", vì AutoMigrate chạy TRƯỚC file này nên khối IF NOT EXISTS
			// column sẽ không bao giờ tới) nên idempotent; ứng viên đang chờ thật (giữ
			// TEACHER_APPLICANT, chưa có TEACHER) không bị chạm tới.
			name: "backfill approved for teacher_profiles of existing TEACHER users (phase-3 approval)",
			sql: `
				UPDATE teacher_profiles tp
				SET approval_status = 'approved'
				WHERE tp.approval_status = 'pending'
				  AND tp.deleted_at IS NULL
				  AND EXISTS (
					SELECT 1 FROM user_system_roles usr
					JOIN system_roles sr ON sr.id = usr.system_role_id
					WHERE usr.user_id = tp.user_id
					  AND usr.status = 'active'
					  AND usr.deleted_at IS NULL
					  AND sr.name = 'TEACHER'
				  );
			`,
		},
		{
			// Phase 4 rút tiền giảng viên (2026-09-28): cột lý do từ chối trên instructor_payouts.
			// AutoMigrate đã thêm từ model; khối này là lớp phòng thủ tường minh, idempotent.
			name: "add rejection_reason column to instructor_payouts (phase-4 withdrawal)",
			sql: `
				DO $$
				BEGIN
					IF NOT EXISTS (SELECT 1 FROM information_schema.columns
					               WHERE table_schema = current_schema() AND table_name='instructor_payouts'
					                 AND column_name='rejection_reason') THEN
						ALTER TABLE instructor_payouts ADD COLUMN rejection_reason TEXT;
					END IF;
				END $$;
			`,
		},
		{
			// Phase 4: đổi CHECK status của instructor_payouts sang model.PayoutStatuses
			// (pending/approved/rejected/completed, bỏ processing/failed). Bảng chưa từng có luồng
			// ghi nào trước Phase 4 nên không cần backfill. SQL sinh từ slice SSOT.
			name: "sync chk_instructor_payouts_status to model.PayoutStatuses (phase-4 withdrawal)",
			sql: buildCheckConstraintSQL("instructor_payouts", "chk_instructor_payouts_status",
				"status", model.PayoutStatuses),
		},
		{
			// QA vòng 2 lane E (Q4): phụ huynh gửi yêu cầu liên kết con. CHECK sinh từ SSOT.
			name: "chk_parent_link_requests_status (qa-r2 lane E)",
			sql: buildCheckConstraintSQL("parent_link_requests", "chk_parent_link_requests_status",
				"status", model.ParentLinkRequestStatuses),
		},
		{
			// DB đã chạy bản đầu của PR #81 (yêu cầu lưu theo student_user_id, chưa có student_email):
			// điền email từ tài khoản và bỏ unique index theo cặp cũ. Không làm gì trên DB mới.
			name: "backfill parent_link_requests.student_email (qa-r2 lane E)",
			sql: `UPDATE parent_link_requests plr SET student_email = LOWER(u.email)
				FROM users u WHERE plr.student_email = '' AND u.id = plr.student_user_id`,
		},
		{
			name: "drop old uq_parent_link_requests_pending (qa-r2 lane E)",
			sql:  `DROP INDEX IF EXISTS uq_parent_link_requests_pending`,
		},
		{
			// Chống spam ở tầng DB: mỗi phụ huynh chỉ có TỐI ĐA 1 yêu cầu đang chờ cho một email. Theo
			// email (không theo student_user_id) vì yêu cầu được lưu cho mọi email, kể cả email không
			// phải học sinh — xem model.ParentLinkRequest.
			name: "uq_parent_link_requests_pending_email (qa-r2 lane E)",
			sql: `CREATE UNIQUE INDEX IF NOT EXISTS uq_parent_link_requests_pending_email
				ON parent_link_requests (parent_user_id, student_email) WHERE status = 'pending'`,
		},
		{
			// Review PR #81 MINOR-8: DB cũ lỡ có status ngoài SSOT (hoặc NULL) thì ADD CONSTRAINT bên dưới
			// sẽ lỗi và API không khởi động. Chuẩn hoá trước: giá trị lạ coi như đã huỷ — an toàn hơn
			// là coi như active (không ai được xem dữ liệu con vì một dòng rác).
			name: "normalize parent_student_relations.status before CHECK (qa-r2 lane E)",
			sql:  buildNormalizeStatusSQL("parent_student_relations", "status", model.ParentStudentStatusRevoked, model.ParentStudentRelationStatuses),
		},
		{
			// Huỷ liên kết nay ghi 'revoked' thật — ràng buộc giá trị cột bằng SSOT.
			name: "chk_parent_student_relations_status (qa-r2 lane E)",
			sql: buildCheckConstraintSQL("parent_student_relations", "chk_parent_student_relations_status",
				"status", model.ParentStudentRelationStatuses),
		},
		{
			// Ai huỷ liên kết (NULL = chưa huỷ, CHECK IN cho qua NULL).
			name: "chk_parent_student_relations_revoked_by (qa-r2 lane E)",
			sql: buildCheckConstraintSQL("parent_student_relations", "chk_parent_student_relations_revoked_by",
				"revoked_by", model.RelationRevokedByValues),
		},
		{
			// Mỗi cặp phụ huynh-học sinh chỉ 1 dòng quan hệ: liên kết lại sau khi huỷ phải KÍCH HOẠT
			// lại dòng cũ, không chèn dòng thứ hai (FindByParentAndStudent lấy First không ORDER, 2 dòng
			// active/revoked cùng cặp sẽ cho kết quả ngẫu nhiên). Chỉ tạo index khi dữ liệu hiện có
			// chưa trùng, để DB dev cũ lỡ có bản ghi trùng không làm API chết lúc khởi động; khi đó
			// log NOTICE để người vận hành dọn tay.
			name: "uq_parent_student_relations_pair (qa-r2 lane E)",
			sql: `
				DO $$
				BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM parent_student_relations
						GROUP BY parent_user_id, student_user_id HAVING count(*) > 1
					) THEN
						CREATE UNIQUE INDEX IF NOT EXISTS uq_parent_student_relations_pair
							ON parent_student_relations (parent_user_id, student_user_id);
					ELSE
						RAISE NOTICE 'parent_student_relations có cặp trùng, bỏ qua uq_parent_student_relations_pair';
					END IF;
				END $$;
			`,
		},
		{
			// S6 backfill: rời/kick/ban nhóm trước đây không đặt conversation_participants.left_at nên
			// người đã rời hoặc bị cấm vẫn đọc/gửi được tin nhóm. Gỡ mọi participant của hội thoại nhóm
			// mà không còn là thành viên ACTIVE của nhóm. Idempotent: chỉ chạm dòng left_at IS NULL, và
			// đường thêm thành viên (addToGroupConversation) luôn tạo/khôi phục cả hai bên cùng lúc.
			name: "backfill conversation_participants.left_at cho thanh vien nhom da roi/bi cam (S6)",
			sql: `
				UPDATE conversation_participants cp
				SET left_at = NOW(), unread_count = 0
				FROM conversations c
				WHERE cp.conversation_id = c.id
				  AND c.group_id IS NOT NULL
				  AND cp.left_at IS NULL
				  AND NOT EXISTS (
					SELECT 1 FROM group_members gm
					WHERE gm.group_id = c.group_id
					  AND gm.user_id = cp.user_id
					  AND gm.status = 'ACTIVE'
					  AND gm.deleted_at IS NULL
				  );
			`,
		},
		{
			// Bạn bè/nhóm (plan 260930): thêm friend_request, friend_accepted, group_added vào CHECK loại thông
			// báo. DB đã có constraint cũ (AutoMigrate chỉ tạo khi thiếu theo TÊN), nên đồng bộ theo
			// model.NotificationTypes. Dùng mẫu NOT VALID + VALIDATE của buildCheckConstraintSQL: rollback
			// về bản backend cũ (không biết 3 giá trị này) vẫn khởi động được dù đã có dòng mang giá trị mới.
			name: "sync chk_notifications_notification_type to model.NotificationTypes (friends/groups)",
			sql: buildCheckConstraintSQL("notifications", "chk_notifications_notification_type",
				"notification_type", model.NotificationTypes),
		},
		{
			name: "chk_friendships_status (friends)",
			sql: buildCheckConstraintSQL("friendships", "chk_friendships_status",
				"status", model.FriendshipStatuses),
		},
		{
			// Mỗi CẶP học viên đúng một dòng, bất kể chiều gửi: chặn trùng hai chiều (A->B và B->A cùng lúc)
			// ở tầng DB. GORM không khai báo được index biểu thức nên tạo ở đây.
			name: "uq_friendships_pair (friends)",
			sql: `CREATE UNIQUE INDEX IF NOT EXISTS uq_friendships_pair
				ON friendships (LEAST(requester_id, addressee_id), GREATEST(requester_id, addressee_id))`,
		},
	}
	statements = append(statements, contestPostMigrations()...)

	for _, stmt := range statements {
		if err := db.Exec(stmt.sql).Error; err != nil {
			return fmt.Errorf("post-migration %q failed: %w", stmt.name, err)
		}
	}
	// Gán chủ cho quiz cũ (quizzes.created_by), xem quiz_created_by_backfill.go.
	if err := runQuizCreatedByBackfill(db); err != nil {
		return err
	}
	// Gán chủ cho lớp cũ (classes.created_by), xem class_created_by_backfill.go.
	if err := runClassCreatedByBackfill(db); err != nil {
		return err
	}
	// Khuyến mãi cũ không hợp lệ (<= 0 hoặc >= giá) về NULL, xem course_discount_price_cleanup.go.
	if err := runCourseDiscountPriceCleanup(db); err != nil {
		return err
	}
	// Chép mã thanh toán của đơn chưa hoàn tất sang orders.payment_code, xem order_payment_code_backfill.go.
	if err := runOrderPaymentCodeBackfill(db); err != nil {
		return err
	}

	// Đổi user_name cũ của tài khoản Google đang trùng prefix email (issue #105), xem google_user_name_backfill.go.
	if err := runGoogleUserNameBackfill(db); err != nil {
		return err
	}

	// Sửa một lần dữ liệu cũ của lane R2 (hội thoại nhóm mồ côi, user_name chứa '@'), xem r2_social_privacy_cleanup.go.
	if err := runR2SocialPrivacyCleanup(db); err != nil {
		return err
	}

	logNotValidCheckConstraints(db)

	log.Printf("Post-migrations applied: %d statement group(s)\n", len(statements))
	return nil
}
