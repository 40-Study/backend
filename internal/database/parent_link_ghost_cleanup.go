package database

// parent_link_ghost_cleanup.go — dọn các yêu cầu liên kết "ma" do thiết kế cũ (PR #81 MAJOR-1) để lại.
// Trước quyết định D8, mọi email hợp lệ đều tạo một dòng `pending` kể cả khi email không thuộc tài
// khoản học sinh nào; không ai trả lời được các dòng đó và phụ huynh thấy chúng chờ mãi. Nay email
// như vậy nhận 404 STUDENT_NOT_FOUND và không tạo dòng (ParentLinkService.CreateRequest), còn các
// dòng cũ được huỷ MỀM ở đây: status='cancelled', KHÔNG xoá (D9) để phụ huynh thấy yêu cầu đã đóng
// và dữ liệu khôi phục được từ các id đã log.
//
// Dòng ma = status 'pending' AND student_user_id IS NULL AND email không khớp tài khoản nào đang giữ
// vai STUDENT (cùng điều kiện với ParentLinkRequestRepository.FindStudentIDByEmailCI). Dòng có email
// nay đã khớp một học sinh thật (đăng ký sau) được GIỮ để học sinh đó còn trả lời.
//
// CHẠY ĐÚNG MỘT LẦN trên mỗi DB (runDataMigrationOnce, dấu trong data_migrations; QA 261009 L4). Chạy mỗi lần khởi động
// thì một yêu cầu pending hợp lệ mà tài khoản học sinh lúc đó tạm thời không có vai STUDENT active (vd đang được cấp lại
// vai) bị huỷ vĩnh viễn; sau D8 không còn dòng ma mới nào được tạo nên một lần là đủ. Câu UPDATE bản thân vẫn idempotent
// (chỉ chạm dòng pending).
// Khôi phục: UPDATE parent_link_requests SET status='pending' WHERE id IN (<id đã log>) AND responded_at IS NULL.

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
)

// parentLinkGhostCleanupName: tên bản ghi đánh dấu trong data_migrations (runDataMigrationOnce).
const parentLinkGhostCleanupName = "parent_link_ghost_cleanup"

// parentLinkGhostLogCap giới hạn số id ghi vào log mỗi lần chạy (số dòng đã huỷ vẫn ghi đủ).
const parentLinkGhostLogCap = 200

// Một câu UPDATE ... RETURNING là một transaction nguyên tử: các id log ra chính là các dòng đã huỷ,
// không có khoảng hở giữa chọn và cập nhật như hai câu SELECT rồi UPDATE.
const parentLinkGhostCleanupSQL = `
	UPDATE parent_link_requests plr
	SET status = 'cancelled', updated_at = now()
	WHERE plr.status = 'pending'
	  AND plr.student_user_id IS NULL
	  AND NOT EXISTS (
		SELECT 1 FROM users u
		WHERE LOWER(u.email) = LOWER(plr.student_email) AND u.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1 FROM user_system_roles usr
			JOIN system_roles sr ON sr.id = usr.system_role_id
			WHERE usr.user_id = u.id AND usr.status = 'active' AND usr.deleted_at IS NULL
			  AND sr.deleted_at IS NULL AND sr.name = 'STUDENT'
		  )
	  )
	RETURNING plr.id;
`

// runParentLinkGhostCleanup chạy sau các bước parent_link_requests trong RunPostMigrations (cần cột
// student_email đã backfill và CHECK status đã có), đúng một lần trên mỗi DB — xem chú thích đầu file.
func runParentLinkGhostCleanup(db *gorm.DB) error {
	if err := runDataMigrationOnce(db, parentLinkGhostCleanupName, cancelGhostParentLinkRequests); err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "soft-cancel ghost parent_link_requests", err)
	}
	return nil
}

// cancelGhostParentLinkRequests huỷ mềm mọi dòng ma và log id đã huỷ (không log gì nếu không có dòng nào).
func cancelGhostParentLinkRequests(tx *gorm.DB) error {
	var ids []string
	if err := tx.Raw(parentLinkGhostCleanupSQL).Scan(&ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	logged, suffix := ids, ""
	if len(ids) > parentLinkGhostLogCap {
		logged, suffix = ids[:parentLinkGhostLogCap], fmt.Sprintf(" (first %d ids)", parentLinkGhostLogCap)
	}
	log.Printf("[migrate] parent_link_requests: cancelled %d ghost pending request(s)%s: %s", len(ids), suffix, strings.Join(logged, ","))
	return nil
}
