package seeds

// Seed demo diễn đàn + hỏi đáp bài học trên Postgres THẬT trong schema tạm (pgtest.IsolatedSchema).
// Không có Postgres: Skip ở local, FAIL khi CI=true.

import (
	"context"
	"testing"

	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

// TestSeedDemoDiscussions_Postgres_ForumVaHoiDapKhopSoThat:
//   - Forum list (ListForumPosts — cùng query API dùng) có 10 bài, đủ 5 category, 2 bài ghim.
//   - reply_count/upvote_count khớp bảng thật (kể cả khi bộ đếm bị làm hỏng trước lần seed sau).
//   - 4 câu hỏi gắn bài học, mỗi câu có trả lời giảng viên; chạy lại không nhân bản.
func TestSeedDemoDiscussions_Postgres_ForumVaHoiDapKhopSoThat(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := laneADemoUsers(t, db, "admin@demo.com", "teacher1@demo.com", "teacher2@demo.com",
		"student1@demo.com", "student2@demo.com", "parent1@demo.com")
	courses := seedDemoCoursesOnce(t, s, users)
	repo := repository.NewDiscussionRepository(db)

	var firstDiscussions, firstVotes int64
	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoDiscussions(users, courses); err != nil {
			t.Fatalf("lần %d: seed discussions: %v", run, err)
		}
		nd, nv := countRows(t, db, &model.Discussion{}), countRows(t, db, &model.DiscussionVote{})
		if run == 1 {
			firstDiscussions, firstVotes = nd, nv
		} else if nd != firstDiscussions || nv != firstVotes {
			t.Errorf("lần 2: %d discussion / %d vote, muốn %d / %d — seed lại không được nhân bản", nd, nv, firstDiscussions, firstVotes)
		}

		posts, total, err := repo.ListForumPosts(context.Background(), "", 1, 50)
		if err != nil {
			t.Fatalf("list forum: %v", err)
		}
		if total != 10 {
			t.Errorf("lần %d: forum có %d bài gốc, muốn 10", run, total)
		}
		categories, pinned := map[string]int{}, 0
		for _, p := range posts {
			categories[*p.Category]++
			if p.IsPinned {
				pinned++
			}
			if p.Slug == nil || *p.Slug == "" || p.Title == nil {
				t.Errorf("bài %s thiếu slug/title", p.ID)
			}
		}
		for _, c := range []string{"programming", "design", "learning-tips", "project", "qna"} {
			if categories[c] == 0 {
				t.Errorf("lần %d: category %s không có bài nào", run, c)
			}
		}
		if pinned != 2 {
			t.Errorf("lần %d: %d bài ghim, muốn 2", run, pinned)
		}

		var mismatched int64
		db.Raw(`SELECT COUNT(*) FROM discussions d WHERE
			reply_count <> (SELECT COUNT(*) FROM discussions r WHERE r.parent_id = d.id AND r.deleted_at IS NULL)
			OR upvote_count <> (SELECT COALESCE(SUM(CASE WHEN v.vote_type = 'upvote' THEN 1 ELSE -1 END), 0)
			                    FROM discussion_votes v WHERE v.discussion_id = d.id)`).Scan(&mismatched)
		if mismatched != 0 {
			t.Errorf("lần %d: %d discussion có reply_count/upvote_count lệch dữ liệu thật", run, mismatched)
		}

		var lessonQs []model.Discussion
		db.Where("lesson_id IS NOT NULL AND parent_id IS NULL").Find(&lessonQs)
		if len(lessonQs) != 4 {
			t.Errorf("lần %d: %d câu hỏi theo bài học, muốn 4", run, len(lessonQs))
		}
		for _, q := range lessonQs {
			var answered int64
			db.Model(&model.Discussion{}).Where("parent_id = ? AND is_instructor_answer = true", q.ID).Count(&answered)
			if answered == 0 {
				t.Errorf("câu hỏi %s chưa có trả lời giảng viên", *q.Slug)
			}
		}

		// Làm hỏng bộ đếm để lần seed sau phải tính lại từ dữ liệu thật.
		if run == 1 {
			db.Exec("UPDATE discussions SET reply_count = 99, upvote_count = -7")
		}
	}
}
