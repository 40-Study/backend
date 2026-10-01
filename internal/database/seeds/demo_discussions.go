package seeds

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// SeedDemoDiscussions seed dữ liệu cho 2 nơi trên web:
//   - Trang Diễn đàn (GET /api/discussions — chỉ lấy lesson_id IS NULL, parent_id IS NULL,
//     is_hidden=false): 10 bài gốc trải đủ 5 category hợp lệ (programming, design, learning-tips,
//     project, qna), 2 bài ghim, mỗi bài 1-3 trả lời (có trả lời giảng viên) và vote lên/xuống.
//   - Panel "Hỏi đáp" trong trình phát bài học (GET /lessons/:id/discussions): 4 câu hỏi của
//     student1 gắn bài học thuộc khoá đã ghi danh, mỗi câu có trả lời của giảng viên khoá.
//
// reply_count/upvote_count được TÍNH LẠI từ bảng thật sau khi seed (upvote_count = số upvote trừ
// downvote, đúng quy ước DiscussionService.VoteDiscussion). Idempotent theo slug bài gốc, cặp
// (bài gốc, tác giả, nội dung) của trả lời và cặp (user, bài) của vote.
func (s *Seeder) SeedDemoDiscussions(users map[string]model.User, courses map[string]model.Course) error {
	log.Println("Seeding demo discussions...")
	var seeded []uuid.UUID
	for _, spec := range demoForumPosts {
		ids, err := s.seedDiscussionThread(spec, users, nil)
		if err != nil {
			return err
		}
		seeded = append(seeded, ids...)
	}
	for _, spec := range demoLessonQuestions {
		lessonID, err := s.findDemoLessonID(courses, spec.CourseSlug, spec.LessonTitle)
		if err != nil {
			return err
		}
		ids, err := s.seedDiscussionThread(spec, users, &lessonID)
		if err != nil {
			return err
		}
		seeded = append(seeded, ids...)
	}
	if err := s.syncDiscussionCounters(seeded); err != nil {
		return err
	}
	log.Printf("Seeded %d forum posts + %d lesson questions\n", len(demoForumPosts), len(demoLessonQuestions))
	return nil
}

// findDemoLessonID tra bài học theo tiêu đề trong đúng khoá (thứ tự thật của khoá) — không dựa vào
// UUID cố định vì course/lesson có thể được seed lại.
func (s *Seeder) findDemoLessonID(courses map[string]model.Course, courseSlug, title string) (uuid.UUID, error) {
	course, found := courses[courseSlug]
	if !found {
		return uuid.Nil, fmt.Errorf("demo discussions: thiếu khoá %s", courseSlug)
	}
	lessons, err := s.courseLessonsInOrder(course.ID)
	if err != nil {
		return uuid.Nil, err
	}
	for _, l := range lessons {
		if l.Title == title {
			return l.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("demo discussions: khoá %s không có bài %q", courseSlug, title)
}

// seedDiscussionThread tạo bài gốc + trả lời + vote, trả về id của mọi bản ghi thuộc thread.
func (s *Seeder) seedDiscussionThread(spec discussionPostSpec, users map[string]model.User, lessonID *uuid.UUID) ([]uuid.UUID, error) {
	author, err := demoDiscussionUser(users, spec.Author)
	if err != nil {
		return nil, err
	}
	createdAt := time.Now().AddDate(0, 0, -spec.DaysAgo)
	root := model.Discussion{UserID: author, LessonID: lessonID, Content: spec.Content, Title: ptr(spec.Title),
		Category: ptr(spec.Category), Slug: ptr(spec.Slug), IsPinned: spec.Pinned}
	root.CreatedAt, root.UpdatedAt = createdAt, createdAt
	if spec.VideoSecond > 0 {
		root.VideoTimestampSecs = ptr(spec.VideoSecond)
	}
	// Unscoped: bài gốc bị người dùng xoá mềm vẫn giữ slug (unique) — không tạo lại, không lỗi trùng.
	if err := s.db.Unscoped().Where("slug = ?", spec.Slug).Attrs(root).FirstOrCreate(&root).Error; err != nil {
		return nil, fmt.Errorf("seed discussion %s: %w", spec.Slug, err)
	}
	if err := s.seedDiscussionVotes(root.ID, spec.Votes, users); err != nil {
		return nil, err
	}
	ids := []uuid.UUID{root.ID}
	for _, r := range spec.Replies {
		replyAuthor, err := demoDiscussionUser(users, r.Author)
		if err != nil {
			return nil, err
		}
		at := createdAt.Add(time.Duration(r.HoursAfter) * time.Hour)
		reply := model.Discussion{UserID: replyAuthor, ParentID: &root.ID, Content: r.Content, IsInstructorAnswer: r.Instructor}
		reply.CreatedAt, reply.UpdatedAt = at, at
		if err := s.db.Where("parent_id = ? AND user_id = ? AND content = ?", root.ID, replyAuthor, r.Content).
			Attrs(reply).FirstOrCreate(&reply).Error; err != nil {
			return nil, fmt.Errorf("seed reply cho %s: %w", spec.Slug, err)
		}
		if err := s.seedDiscussionVotes(reply.ID, r.Votes, users); err != nil {
			return nil, err
		}
		ids = append(ids, reply.ID)
	}
	return ids, nil
}

func (s *Seeder) seedDiscussionVotes(discussionID uuid.UUID, votes map[string]string, users map[string]model.User) error {
	for email, voteType := range votes {
		voter, err := demoDiscussionUser(users, email)
		if err != nil {
			return err
		}
		v := model.DiscussionVote{UserID: voter, DiscussionID: discussionID, VoteType: voteType}
		if err := s.db.Where("user_id = ? AND discussion_id = ?", voter, discussionID).
			Attrs(v).FirstOrCreate(&v).Error; err != nil {
			return fmt.Errorf("seed discussion vote: %w", err)
		}
	}
	return nil
}

// syncDiscussionCounters ghi lại reply_count (số trả lời chưa xoá) và upvote_count (upvote -
// downvote) từ dữ liệu thật, để số trên danh sách khớp khi mở chi tiết bài.
func (s *Seeder) syncDiscussionCounters(ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	err := s.db.Exec(`UPDATE discussions d SET
		reply_count = (SELECT COUNT(*) FROM discussions r WHERE r.parent_id = d.id AND r.deleted_at IS NULL),
		upvote_count = (SELECT COALESCE(SUM(CASE WHEN v.vote_type = 'upvote' THEN 1 ELSE -1 END), 0)
		                FROM discussion_votes v WHERE v.discussion_id = d.id)
		WHERE d.id IN ?`, ids).Error
	if err != nil {
		return fmt.Errorf("sync discussion counters: %w", err)
	}
	return nil
}

func demoDiscussionUser(users map[string]model.User, email string) (uuid.UUID, error) {
	u := users[email]
	if u.ID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("demo discussions: thiếu tài khoản %s", email)
	}
	return u.ID, nil
}
