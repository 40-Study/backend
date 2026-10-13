package database

// forum_reply_count_backfill.go — QA2 F-05: discussions.reply_count của các bài gốc đã lệch khỏi số trả lời còn
// sống (xoá trả lời trước đây không trừ bộ đếm; dữ liệu nạp trực tiếp không qua AddComment không cộng). Đường ghi
// mới giữ bộ đếm đúng (DiscussionRepository.DeletePost); bước này sửa các dòng CŨ.
//
// Chỉ chạm bài gốc (parent_id IS NULL, cả bài diễn đàn lẫn câu hỏi theo bài học) và chỉ dòng đang lệch, nên chạy
// lại là no-op. Biểu thức đếm dùng chung với đường xoá: model.DiscussionLiveReplyCountSQL.
//
// CHẠY ĐÚNG MỘT LẦN trên mỗi DB (runDataMigrationOnce, dấu trong data_migrations).

import (
	"fmt"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

const forumReplyCountBackfillName = "forum_reply_count_backfill"

const forumReplyCountBackfillSQL = `
	UPDATE discussions d
	SET reply_count = ` + model.DiscussionLiveReplyCountSQL + `
	WHERE d.parent_id IS NULL
	  AND d.reply_count IS DISTINCT FROM ` + model.DiscussionLiveReplyCountSQL

// backfillForumReplyCounts chạy câu SQL trên tx và trả số dòng đã sửa (test dùng để kiểm "lần hai sửa 0").
func backfillForumReplyCounts(tx *gorm.DB) (int64, error) {
	res := tx.Exec(forumReplyCountBackfillSQL)
	return res.RowsAffected, res.Error
}

// runForumReplyCountBackfill — xem chú thích đầu file. Gọi từ RunPostMigrations sau AutoMigrate.
func runForumReplyCountBackfill(db *gorm.DB) error {
	err := runDataMigrationOnce(db, forumReplyCountBackfillName, func(tx *gorm.DB) error {
		_, err := backfillForumReplyCounts(tx)
		return err
	})
	if err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "backfill discussions.reply_count", err)
	}
	return nil
}
