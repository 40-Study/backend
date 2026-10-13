package database

// Backfill discussions.reply_count (QA2 F-05) trên Postgres thật, schema tạm. Mutation đã kiểm: bỏ lọc
// deleted_at (đếm cả trả lời đã xoá), bỏ `parent_id IS NULL` (chạm cả trả lời), hoặc bỏ runDataMigrationOnce
// (chạy lại mỗi lần khởi động) làm test ĐỎ.

import (
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestForumReplyCountBackfill_FixesDriftedCounters_RunsOnce(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	author := backfillUser(t, db, "forum")

	root := func(count int) uuid.UUID {
		d := model.Discussion{UserID: author, Content: "gốc", ReplyCount: count}
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("tạo bài gốc: %v", err)
		}
		return d.ID
	}
	reply := func(parent uuid.UUID, deleted bool) uuid.UUID {
		d := model.Discussion{UserID: author, Content: "trả lời", ParentID: &parent}
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("tạo trả lời: %v", err)
		}
		if deleted {
			if err := db.Delete(&model.Discussion{}, "id = ?", d.ID).Error; err != nil {
				t.Fatalf("xoá mềm trả lời: %v", err)
			}
		}
		return d.ID
	}
	countOf := func(id uuid.UUID) int {
		var d model.Discussion
		if err := db.Unscoped().Where("id = ?", id).First(&d).Error; err != nil {
			t.Fatalf("đọc discussion: %v", err)
		}
		return d.ReplyCount
	}

	zeroButHasReplies := root(0) // dữ liệu nạp trực tiếp: 3 trả lời mà bộ đếm 0
	reply(zeroButHasReplies, false)
	reply(zeroButHasReplies, false)
	reply(zeroButHasReplies, false)

	tooHigh := root(5) // xoá trả lời không từng trừ: 5 mà còn 1 sống
	reply(tooHigh, false)
	reply(tooHigh, true)
	reply(tooHigh, true)

	nested := root(0) // 1 trả lời + 1 trả lời lồng dưới nó: AddComment tính cả hai vào bài gốc
	top := reply(nested, false)
	reply(top, false)

	underDeleted := root(0) // trả lời lồng dưới một trả lời đã xoá không còn hiện ở đâu: không đếm
	deletedTop := reply(underDeleted, true)
	reply(deletedTop, false)

	alreadyRight := root(1)
	reply(alreadyRight, false)
	noReplies := root(4)
	replyCount := reply(zeroButHasReplies, false) // một trả lời: bộ đếm của CHÍNH nó không phải việc của bước này
	if err := db.Exec("UPDATE discussions SET reply_count = 9 WHERE id = ?", replyCount).Error; err != nil {
		t.Fatalf("đặt số cho trả lời: %v", err)
	}

	// Migrate đã ghi dấu trên DB trống: xoá dấu để mô phỏng DB cũ khởi động lần đầu với bản mới.
	if err := db.Exec("DELETE FROM data_migrations WHERE name = ?", forumReplyCountBackfillName).Error; err != nil {
		t.Fatalf("xoá dấu backfill: %v", err)
	}
	if err := runForumReplyCountBackfill(db); err != nil {
		t.Fatalf("backfill lần 1: %v", err)
	}
	for name, c := range map[string]struct {
		id   uuid.UUID
		want int
	}{
		"bộ đếm 0 mà có trả lời":           {zeroButHasReplies, 4},
		"bộ đếm quá cao (xoá không trừ)":   {tooHigh, 1},
		"trả lời lồng tính vào bài gốc":    {nested, 2},
		"dưới trả lời đã xoá không tính":   {underDeleted, 0},
		"đã đúng thì giữ nguyên":           {alreadyRight, 1},
		"bài gốc không có trả lời nào":     {noReplies, 0},
		"trả lời (không phải bài gốc) giữ": {replyCount, 9},
	} {
		if got := countOf(c.id); got != c.want {
			t.Errorf("%s: reply_count = %d, muốn %d", name, got, c.want)
		}
	}

	// Chạy lại câu SQL: no-op (idempotent). Và bước chỉ chạy một lần: bài lệch mới sau đó không bị chạm nữa.
	if n, err := backfillForumReplyCounts(db); err != nil || n != 0 {
		t.Fatalf("câu SQL lần 2 sửa %d dòng (err=%v), muốn 0", n, err)
	}
	later := root(7)
	if err := runForumReplyCountBackfill(db); err != nil {
		t.Fatalf("backfill lần 3: %v", err)
	}
	if got := countOf(later); got != 7 {
		t.Errorf("bước đã ghi dấu không được chạy lại: bài mới có reply_count = %d, muốn giữ 7", got)
	}
}

// Wired, not just present: RunPostMigrations (đường API khởi động chạy) phải gọi bước này.
func TestRunPostMigrations_RunsForumReplyCountBackfillOnOldDB(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	author := backfillUser(t, db, "forum-wire")
	root := model.Discussion{UserID: author, Content: "gốc"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("tạo bài gốc: %v", err)
	}
	if err := db.Create(&model.Discussion{UserID: author, Content: "tl", ParentID: &root.ID}).Error; err != nil {
		t.Fatalf("tạo trả lời: %v", err)
	}
	if err := db.Exec("DELETE FROM data_migrations WHERE name = ?", forumReplyCountBackfillName).Error; err != nil {
		t.Fatalf("xoá dấu backfill: %v", err)
	}
	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}
	var got model.Discussion
	if err := db.Where("id = ?", root.ID).First(&got).Error; err != nil {
		t.Fatalf("đọc bài gốc: %v", err)
	}
	if got.ReplyCount != 1 {
		t.Errorf("RunPostMigrations phải sửa reply_count 0 -> 1, nhận %d", got.ReplyCount)
	}
}
